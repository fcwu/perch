package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// Required for Chrome's Private Network Access policy: browsers that resolve
	// a public FQDN to a private IP block ws:// upgrades without this header.
	EnableCompression: false,
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// Chrome Private Network Access: allow ws:// from pages whose public FQDN
	// resolves to a private IP (e.g. cdrdla.myqnapcloud.com → 172.x.x.x).
	// Must be in the responseHeader arg, not w.Header(), because gorilla hijacks
	// the connection and writes the 101 response directly from responseHeader.
	h := http.Header{"Access-Control-Allow-Private-Network": []string{"true"}}
	return upgrader.Upgrade(w, r, h)
}

type Server struct {
	pty             *PTYManager
	auth            *AuthMiddleware
	im              *IMManager
	sessions        SessionProvider
	userSessions    *UserSessionManager
	gitlabAuth      *gitLabAuth
	adminAuth       *adminAuth
	adminHub        *AdminHub
	store           *Store
	userRateLimiter *UserRateLimiter
	mode            OperatingMode
	logger          *slog.Logger
	mux             *http.ServeMux
}

func newServer(pm *PTYManager, auth *AuthMiddleware, im *IMManager, sessions SessionProvider, userSessions *UserSessionManager, gitlabAuth *gitLabAuth, adminAuth *adminAuth, adminHub *AdminHub, store *Store, userRL *UserRateLimiter, logger *slog.Logger) *Server {
	return newServerWithMode(pm, auth, im, sessions, userSessions, gitlabAuth, adminAuth, adminHub, store, userRL, ModeSingle, logger)
}

func newServerWithMode(pm *PTYManager, auth *AuthMiddleware, im *IMManager, sessions SessionProvider, userSessions *UserSessionManager, gitlabAuth *gitLabAuth, adminAuth *adminAuth, adminHub *AdminHub, store *Store, userRL *UserRateLimiter, mode OperatingMode, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{pty: pm, auth: auth, im: im, sessions: sessions, userSessions: userSessions, gitlabAuth: gitlabAuth, adminAuth: adminAuth, adminHub: adminHub, store: store, userRateLimiter: userRL, mode: mode, logger: logger, mux: http.NewServeMux()}
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/input", s.handleInput)
	s.mux.HandleFunc("/sessions", s.handleSessions)
	s.mux.HandleFunc("/ws/session", s.handleSessionWS)
	// /hook is called by Claude Code hooks running inside the container (localhost).
	// It is intentionally exempt from auth — registered on the raw mux, bypassing the
	// auth wrapper applied in ServeHTTP.
	s.mux.Handle("/hook", newHookHandler(im, userSessions))
	if auth != nil && auth.mode == "password" {
		s.mux.HandleFunc("/login", auth.handleLogin)
	}
	// Public auth endpoints.
	if gitlabAuth != nil {
		s.mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
			gitlabAuth.handleAuthStatus(w, r, mode, auth)
		})
		s.mux.HandleFunc("/auth/logout", gitlabAuth.handleLogout)
		s.mux.HandleFunc("/api/logout", gitlabAuth.handleLogout)
	}
	// Admin endpoints — POST handled by adminAuth, GET falls through to SPA below.
	adminLoginHandler := adminAuth.handleLogin
	if adminAuth != nil {
		adminMW := adminAuth.middleware
		s.mux.Handle("/admin/history/", adminMW(http.HandlerFunc(s.handleAdminHistoryDetail)))
		s.mux.Handle("/admin/history", adminMW(http.HandlerFunc(s.handleAdminHistory)))
		s.mux.Handle("/admin/analytics", adminMW(http.HandlerFunc(s.handleAdminAnalytics)))
		s.mux.Handle("/ws/admin", adminMW(http.HandlerFunc(s.handleAdminWS)))
	}
	// GitLab OAuth endpoints (no auth required).
	// Chat routes use GitLab middleware only when GitLab auth is active AND
	// the primary auth method is not password (password sessions use a different
	// cookie and would be rejected by the GitLab middleware).
	gitlabChatAuth := gitlabAuth != nil && gitlabAuth.enabled() && (auth == nil || auth.mode != "password")
	if gitlabAuth != nil && gitlabAuth.enabled() {
		s.mux.HandleFunc("/auth/gitlab", gitlabAuth.handleRedirect)
		s.mux.HandleFunc("/auth/callback", gitlabAuth.handleCallback)
	}
	if gitlabChatAuth {
		// Chat API and SSE stream: protected by GitLab session cookie.
		chatHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatAPI))
		s.mux.Handle("/api/chat", chatHandler)
		chatSSEHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatSSE))
		s.mux.Handle("/api/chat/stream", chatSSEHandler)
		chatWSHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatWS))
		s.mux.Handle("/ws/chat", chatWSHandler)
	} else {
		// Register chat routes even when GitLab auth is disabled so unauthenticated
		// requests get a proper error response instead of falling through to the SPA
		// handler and returning 200 HTML. ServeHTTP applies primary auth before
		// reaching these handlers in password mode.
		s.mux.Handle("/api/chat", http.HandlerFunc(s.handleChatAPI))
		s.mux.Handle("/api/chat/stream", http.HandlerFunc(s.handleChatSSE))
		s.mux.Handle("/ws/chat", http.HandlerFunc(s.handleChatWS))
	}
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		// Serve index.html for SPA routes that don't correspond to static files.
		spaHandler := &spaFileServer{fs: distFS, fileServer: fileServer}
		s.mux.Handle("/", spaHandler)
		// /admin/login GET → SPA, POST → login handler.
		s.mux.HandleFunc("/admin/login", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				adminLoginHandler(w, r)
				return
			}
			serveIndexHTML(w, r, distFS)
		})
		if mode == ModeMulti {
			// Multi-user: /chat and /admin serve index.html without auth — API layer enforces auth.
			s.mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
			s.mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
		} else if gitlabAuth != nil && gitlabAuth.enabled() {
			// Single-user GitLab mode: /chat served without auth (API enforces it).
			s.mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
		}
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Paths always exempt from primary auth middleware.
	switch r.URL.Path {
	case "/hook", "/auth/gitlab", "/auth/callback", "/auth/logout", "/api/logout",
		"/api/auth/status", "/admin/login", "/login":
		s.mux.ServeHTTP(w, r)
		return
	}
	// Admin paths use their own cookie auth or are public (login page, SPA).
	if r.URL.Path == "/ws/admin" || r.URL.Path == "/admin/history" ||
		strings.HasPrefix(r.URL.Path, "/admin/history/") ||
		r.URL.Path == "/admin/analytics" ||
		r.URL.Path == "/admin/login" ||
		r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
		s.mux.ServeHTTP(w, r)
		return
	}
	// Normalise /chat/ → /chat.
	if r.URL.Path == "/chat/" {
		r.URL.Path = "/chat"
	}
	// Chat/API paths: auth enforced at handler registration, skip primary auth.
	// Exception: in password mode the chat API routes must go through primary auth
	// because they are registered without a middleware wrapper (GitLab auth disabled).
	if r.URL.Path == "/chat" || r.URL.Path == "/api/chat" || r.URL.Path == "/api/chat/stream" || r.URL.Path == "/ws/chat" {
		if s.auth != nil && s.auth.mode == "password" && r.URL.Path != "/chat" {
			s.auth.wrap(s.mux).ServeHTTP(w, r)
			return
		}
		s.mux.ServeHTTP(w, r)
		return
	}
	// Static assets and the SPA root load without auth so the login UI can render.
	// Only protect interactive API/WS endpoints under primary auth.
	if s.auth != nil && s.auth.mode == "password" {
		switch r.URL.Path {
		case "/ws", "/input", "/sessions", "/ws/session":
			s.auth.wrap(s.mux).ServeHTTP(w, r)
			return
		}
		s.mux.ServeHTTP(w, r)
		return
	}
	if s.auth != nil {
		s.auth.wrap(s.mux).ServeHTTP(w, r)
	} else {
		s.mux.ServeHTTP(w, r)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrade(w, r)
	if err != nil {
		s.logger.Error("ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	ch, unsub := s.pty.subscribe()
	defer unsub()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range ch {
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var resize struct {
			Type string `json:"type"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
			s.pty.resize(resize.Cols, resize.Rows)
			continue
		}
		if err := s.pty.write(msg); err != nil {
			s.logger.Error("pty write", "err", err)
		}
	}
	<-done
}

func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct{ Data string `json:"data"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.pty.write([]byte(req.Data)); err != nil {
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSessions returns the list of active IM sessions as JSON.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var list []SessionView
	if s.sessions != nil {
		list = s.sessions.ListSessions()
	}
	if list == nil {
		list = []SessionView{}
	}
	json.NewEncoder(w).Encode(list)
}

// handleSessionWS streams PTY output for a Discord channel session (read-only).
// Query param: ?id=<channelID>
func (s *Server) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("id")
	if channelID == "" || s.sessions == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	ch, unsub, ok := s.sessions.SubscribeSession(channelID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	defer unsub()

	conn, err := wsUpgrade(w, r)
	if err != nil {
		s.logger.Error("ws session upgrade", "err", err)
		return
	}
	defer conn.Close()

	// Handle resize messages; forward all other input to the PTY.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var resize struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
				s.sessions.ResizeSession(channelID, resize.Cols, resize.Rows)
			} else {
				if err := s.sessions.WriteSession(channelID, msg); err != nil {
					s.logger.Warn("ws session write", "channel", channelID, "err", err)
				}
			}
		}
	}()

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// serveIndexHTML writes the embedded index.html to the response (SPA entry point).
func serveIndexHTML(w http.ResponseWriter, _ *http.Request, distFS fs.FS) {
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// spaFileServer wraps a standard file server and falls back to index.html for unknown paths.
type spaFileServer struct {
	fs         fs.FS
	fileServer http.Handler
}

func (s *spaFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" {
		s.fileServer.ServeHTTP(w, r)
		return
	}
	// Strip leading slash for fs.Stat lookup.
	if _, err := fs.Stat(s.fs, path[1:]); err != nil {
		serveIndexHTML(w, r, s.fs)
		return
	}
	s.fileServer.ServeHTTP(w, r)
}

// handleChatAPI handles POST /api/chat — starts a new OpenCode session for the authenticated user.
func (s *Server) handleChatAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.userSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID, _ := r.Context().Value(ctxUserID).(string)
	username, _ := r.Context().Value(ctxUsername).(string)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Query           string `json:"query"`
		NewConversation bool   `json:"new_conversation,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		http.Error(w, "bad request: missing query", http.StatusBadRequest)
		return
	}

	// Per-user rate limit check
	if s.userRateLimiter != nil {
		if ok, retryAfter := s.userRateLimiter.Allow(userID); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error":          "rate limit exceeded",
				"retry_after_ms": retryAfter,
			})
			return
		}
	}

	if err := s.userSessions.StartSession(userID, username, req.Query, req.NewConversation); err != nil {
		if ce, ok := err.(interface{ IsConflict() bool }); ok && ce.IsConflict() {
			http.Error(w, "session already in progress", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
}

// handleChatSSE handles GET /api/chat/stream — streams PTY output and JSON tool events via
// Server-Sent Events. This bypasses transparent HTTP proxies that strip WebSocket Upgrade headers.
// PTY bytes are sent as base64-encoded "pty" events; JSON events as "json" events.
func (s *Server) handleChatSSE(w http.ResponseWriter, r *http.Request) {
	if s.userSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID, _ := r.Context().Value(ctxUserID).(string)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ptyCh, ptyUnsub, ok := s.userSessions.SubscribeSession(userID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	defer ptyUnsub()

	jsonCh, jsonUnsub, _ := s.userSessions.SubscribeJSON(userID)
	if jsonUnsub != nil {
		defer jsonUnsub()
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	writeEvent := func(eventType, data string) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
			return false
		}
		if canFlush {
			flusher.Flush()
		}
		return true
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ptyCh:
			if !ok {
				return
			}
			if !writeEvent("pty", base64.StdEncoding.EncodeToString(data)) {
				return
			}
		case msg, ok := <-jsonCh:
			if !ok {
				return
			}
			// Drain any pending PTY data before forwarding this JSON event,
			// so fast responses don't lose output when 'done' races ahead.
			for {
				select {
				case data := <-ptyCh:
					if !writeEvent("pty", base64.StdEncoding.EncodeToString(data)) {
						return
					}
				default:
					goto sendJSON
				}
			}
		sendJSON:
			if !writeEvent("json", msg) {
				return
			}
		}
	}
}

// handleChatWS handles GET /ws/chat — streams PTY output and JSON tool events to the browser.
func (s *Server) handleChatWS(w http.ResponseWriter, r *http.Request) {
	if s.userSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID, _ := r.Context().Value(ctxUserID).(string)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ptyCh, ptyUnsub, ok := s.userSessions.SubscribeSession(userID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	defer ptyUnsub()

	jsonCh, jsonUnsub, _ := s.userSessions.SubscribeJSON(userID)
	if jsonUnsub != nil {
		defer jsonUnsub()
	}

	conn, err := wsUpgrade(w, r)
	if err != nil {
		s.logger.Error("ws chat upgrade", "err", err)
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case data, ok := <-ptyCh:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case msg, ok := <-jsonCh:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// handleAdminWS streams admin session events via WebSocket.
func (s *Server) handleAdminWS(w http.ResponseWriter, r *http.Request) {
	if s.adminHub == nil {
		http.Error(w, "admin not configured", http.StatusServiceUnavailable)
		return
	}
	conn, err := wsUpgrade(w, r)
	if err != nil {
		s.logger.Error("ws admin upgrade", "err", err)
		return
	}
	defer conn.Close()

	ch, unsub := s.adminHub.subscribe()
	defer unsub()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// handleAdminHistory handles GET /admin/history
func (s *Server) handleAdminHistory(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "sessions": []any{}})
		return
	}
	q := r.URL.Query()
	user := q.Get("user")
	keyword := q.Get("q")
	var from, to int64
	fmt.Sscanf(q.Get("from"), "%d", &from)
	fmt.Sscanf(q.Get("to"), "%d", &to)
	var page, limit int
	fmt.Sscanf(q.Get("page"), "%d", &page)
	fmt.Sscanf(q.Get("limit"), "%d", &limit)

	sessions, total, err := s.store.ListSessions(user, keyword, from, to, page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"total": total, "sessions": sessions})
}

// handleAdminHistoryDetail handles GET /admin/history/<id>
func (s *Server) handleAdminHistoryDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/admin/history/"):]
	if id == "" || s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	detail, err := s.store.GetSession(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if detail == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// handleAdminAnalytics handles GET /admin/analytics
func (s *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&UsageStats{Users: []UserStat{}, TopTools: []ToolStat{}})
		return
	}
	q := r.URL.Query()
	var from, to int64
	fmt.Sscanf(q.Get("from"), "%d", &from)
	fmt.Sscanf(q.Get("to"), "%d", &to)

	stats, err := s.store.GetUsageStats(from, to)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
