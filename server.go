package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

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
	userSessions    *UserSessionManager  // per-user PTY session manager (web /ws only)
	chatSessions    ChatSessionManager   // active chat session backend (PTY or ACP)
	gitlabAuth      *gitLabAuth
	adminAuth       *adminAuth
	adminHub        *ManagementHub
	store           *Store
	userRateLimiter *UserRateLimiter
	sm              *SettingsManager
	mode            OperatingMode
	logger          *slog.Logger
	mux             *http.ServeMux

	// defaultRuntime is the server-wide AGENT_RUNTIME, used when a new
	// conversation is created without an explicit runtime/model override.
	defaultRuntime AgentRuntime
	// pool is the ACP session pool, used to evict (user, conv) sessions on
	// runtime/model PATCH so the next prompt boots a fresh subprocess.
	pool *ACPSessionPool
	// scheduler is the chat scheduler, used to hot-reload chat_schedules rows
	// when the schedule CRUD endpoints mutate them.
	scheduler *Scheduler
	// imgStore serves agent-produced images via /api/images/<convID>/<file>.
	imgStore *imageStore
	workdir  string
}

func newServer(pm *PTYManager, auth *AuthMiddleware, im *IMManager, sessions SessionProvider, userSessions *UserSessionManager, gitlabAuth *gitLabAuth, adminAuth *adminAuth, adminHub *ManagementHub, store *Store, userRL *UserRateLimiter, logger *slog.Logger) *Server {
	return newServerWithMode(pm, auth, im, sessions, userSessions, gitlabAuth, adminAuth, adminHub, store, userRL, nil, ModeSingle, logger)
}

func newServerWithMode(pm *PTYManager, auth *AuthMiddleware, im *IMManager, sessions SessionProvider, userSessions *UserSessionManager, gitlabAuth *gitLabAuth, adminAuth *adminAuth, adminHub *ManagementHub, store *Store, userRL *UserRateLimiter, sm *SettingsManager, mode OperatingMode, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{pty: pm, auth: auth, im: im, sessions: sessions, userSessions: userSessions, chatSessions: userSessions, gitlabAuth: gitlabAuth, adminAuth: adminAuth, adminHub: adminHub, store: store, userRateLimiter: userRL, sm: sm, mode: mode, logger: logger, mux: http.NewServeMux()}
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/input", s.handleInput)
	s.mux.HandleFunc("/sessions", s.handleSessions)
	s.mux.HandleFunc("/ws/session", s.handleSessionWS)
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
	if adminAuth != nil {
		managementMW := adminAuth.middleware
		s.mux.Handle("/api/management/history/", managementMW(http.HandlerFunc(s.handleManagementHistoryDetail)))
		s.mux.Handle("/api/management/history", managementMW(http.HandlerFunc(s.handleManagementHistory)))
		s.mux.Handle("/api/management/analytics", managementMW(http.HandlerFunc(s.handleManagementAnalytics)))
		// Read-only admin views over conversations + chat schedules. No PATCH /
		// POST / DELETE routes are registered — operators must use SQL for
		// destructive operations.
		s.mux.Handle("GET /api/management/conversations", managementMW(http.HandlerFunc(s.handleManagementListConversations)))
		s.mux.Handle("GET /api/management/conversations/{id}", managementMW(http.HandlerFunc(s.handleManagementGetConversation)))
		s.mux.Handle("GET /api/management/conversations/{id}/messages", managementMW(http.HandlerFunc(s.handleManagementListConversationMessages)))
		s.mux.Handle("GET /api/management/schedules", managementMW(http.HandlerFunc(s.handleManagementListSchedules)))
		// 405 guards — without these, PATCH/POST/DELETE fall through to the
		// SPA `/` handler and return 200 HTML, which violates the read-only
		// admin contract (admin-conversations-readonly spec §8.6).
		methodNotAllowed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		})
		s.mux.Handle("PATCH /api/management/conversations/{id}", methodNotAllowed)
		s.mux.Handle("POST /api/management/conversations/{id}", methodNotAllowed)
		s.mux.Handle("DELETE /api/management/conversations/{id}", methodNotAllowed)
		s.mux.Handle("PATCH /api/management/schedules/{id}", methodNotAllowed)
		s.mux.Handle("POST /api/management/schedules", methodNotAllowed)
		s.mux.Handle("POST /api/management/schedules/{id}", methodNotAllowed)
		s.mux.Handle("DELETE /api/management/schedules/{id}", methodNotAllowed)
		if mode == ModeMulti {
			s.mux.Handle("/ws/management", managementMW(http.HandlerFunc(s.handleManagementWS)))
		} else {
			s.mux.HandleFunc("/ws/management", http.NotFound)
		}
		s.mux.Handle("GET /api/settings", managementMW(http.HandlerFunc(s.handleGetSettings)))
		s.mux.Handle("PATCH /api/settings", managementMW(http.HandlerFunc(s.handlePatchSettings)))
		s.mux.Handle("POST /api/management/restart", managementMW(http.HandlerFunc(s.handleManagementRestart)))
	}
	// GitLab OAuth endpoints (no auth required).
	// Chat routes use GitLab middleware only when GitLab auth is active AND
	// the primary auth method is not password (password sessions use a different
	// cookie and would be rejected by the GitLab middleware).
	gitlabChatAuth := gitlabAuth != nil && gitlabAuth.enabled() && (auth == nil || auth.mode != "password")
	if gitlabAuth != nil && gitlabAuth.enabled() && (auth == nil || auth.mode != "password") {
		s.mux.HandleFunc("/auth/gitlab", gitlabAuth.handleRedirect)
		s.mux.HandleFunc("/auth/callback", gitlabAuth.handleCallback)
	} else if auth != nil && auth.mode == "password" {
		s.mux.HandleFunc("/auth/gitlab", http.NotFound)
		s.mux.HandleFunc("/auth/callback", http.NotFound)
	}
	if gitlabChatAuth {
		// Chat API and SSE stream: protected by GitLab session cookie.
		chatHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatAPI))
		s.mux.Handle("/api/chat", chatHandler)
		chatSSEHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatSSE))
		s.mux.Handle("/api/chat/stream", chatSSEHandler)
		chatWSHandler := gitlabAuth.middleware(http.HandlerFunc(s.handleChatWS))
		s.mux.Handle("/ws/chat", chatWSHandler)
		// Conversation routes: protected by GitLab session cookie.
		s.mux.Handle("GET /api/conversations", gitlabAuth.middleware(http.HandlerFunc(s.handleListConversations)))
		s.mux.Handle("PATCH /api/conversations/{id}", gitlabAuth.middleware(http.HandlerFunc(s.handlePatchConversation)))
		s.mux.Handle("DELETE /api/conversations/{id}", gitlabAuth.middleware(http.HandlerFunc(s.handleDeleteConversation)))
		s.mux.Handle("GET /api/conversations/{id}/messages", gitlabAuth.middleware(http.HandlerFunc(s.handleListConversationMessages)))
		s.mux.Handle("GET /api/conversations/{id}/schedules", gitlabAuth.middleware(http.HandlerFunc(s.handleListChatSchedules)))
		s.mux.Handle("POST /api/conversations/{id}/schedules", gitlabAuth.middleware(http.HandlerFunc(s.handleCreateChatSchedule)))
		s.mux.Handle("DELETE /api/conversations/{id}/schedules/{job_id}", gitlabAuth.middleware(http.HandlerFunc(s.handleDeleteChatSchedule)))
		// Runtime registry — auth-gated alongside the chat endpoints.
		s.mux.Handle("GET /api/runtimes", gitlabAuth.middleware(http.HandlerFunc(s.handleListRuntimes)))
		// Agent-produced image files — auth-gated.
		s.mux.Handle("GET /api/images/{convID}/{filename}", gitlabAuth.middleware(http.HandlerFunc(s.handleImageFile)))
	} else {
		// Register chat routes even when GitLab auth is disabled so unauthenticated
		// requests get a proper error response instead of falling through to the SPA
		// handler and returning 200 HTML. ServeHTTP applies primary auth before
		// reaching these handlers in password mode.
		s.mux.Handle("/api/chat", http.HandlerFunc(s.handleChatAPI))
		s.mux.Handle("/api/chat/stream", http.HandlerFunc(s.handleChatSSE))
		s.mux.Handle("/ws/chat", http.HandlerFunc(s.handleChatWS))
		// Conversation routes (no extra middleware; ServeHTTP applies primary auth in password mode).
		s.mux.HandleFunc("GET /api/conversations", s.handleListConversations)
		s.mux.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
		s.mux.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
		s.mux.HandleFunc("GET /api/conversations/{id}/messages", s.handleListConversationMessages)
		s.mux.HandleFunc("GET /api/conversations/{id}/schedules", s.handleListChatSchedules)
		s.mux.HandleFunc("POST /api/conversations/{id}/schedules", s.handleCreateChatSchedule)
		s.mux.HandleFunc("DELETE /api/conversations/{id}/schedules/{job_id}", s.handleDeleteChatSchedule)
		s.mux.HandleFunc("GET /api/runtimes", s.handleListRuntimes)
		// Agent-produced image files.
		s.mux.HandleFunc("GET /api/images/{convID}/{filename}", s.handleImageFile)
	}
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		// Serve index.html for SPA routes that don't correspond to static files.
		// / redirects to /chat; other paths fall through to SPA.
		spaHandler := &spaFileServer{fs: distFS, fileServer: fileServer, redirectRoot: true}
		s.mux.Handle("/", spaHandler)
		// /admin/login GET → SPA, POST → login handler.
		s.mux.HandleFunc("/management/login", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && adminAuth != nil {
				adminAuth.handleLogin(w, r)
				return
			}
			serveIndexHTML(w, r, distFS)
		})
		if mode == ModeMulti {
			// Multi-user: /chat and /admin serve index.html without auth — API layer enforces auth.
			s.mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
			s.mux.HandleFunc("/management", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
		} else if gitlabAuth != nil && gitlabAuth.enabled() {
			// Single-user GitLab mode: /chat served without auth (API enforces it).
			s.mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
				serveIndexHTML(w, r, distFS)
			})
		}
		// /terminal: admin-only route serving the SPA.
		// Admin check: cookie-based admin (adminAuth.middleware) in single-user mode,
		// or GitLab role-based in multi-user mode.
		terminalHandler := func(w http.ResponseWriter, r *http.Request) {
			serveIndexHTML(w, r, distFS)
		}
		if mode == ModeMulti && gitlabAuth != nil && gitlabAuth.enabled() {
			// Multi-user GitLab: wrap with role check; redirect non-admin to /chat.
			s.mux.Handle("/terminal", gitlabAuth.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if user has admin role.
				role, _ := r.Context().Value(ctxRole).(string)
				if role != "admin" {
					http.Redirect(w, r, "/chat", http.StatusFound)
					return
				}
				terminalHandler(w, r)
			})))
		} else if adminAuth != nil && adminAuth.enabled() {
			// Single-user: check admin cookie; redirect to /chat if missing.
			s.mux.HandleFunc("/terminal", func(w http.ResponseWriter, r *http.Request) {
				if !adminAuth.isAdmin(r) {
					http.Redirect(w, r, "/chat", http.StatusFound)
					return
				}
				terminalHandler(w, r)
			})
		} else {
			// No admin token configured: allow everyone.
			s.mux.HandleFunc("/terminal", terminalHandler)
		}
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Paths always exempt from primary auth middleware.
	switch r.URL.Path {
	case "/auth/gitlab", "/auth/callback", "/auth/logout", "/api/logout",
		"/api/auth/status", "/management/login", "/login":
		s.mux.ServeHTTP(w, r)
		return
	}
	// Admin paths use their own cookie auth or are public (login page, SPA).
	if r.URL.Path == "/ws/management" ||
		strings.HasPrefix(r.URL.Path, "/api/management/") ||
		r.URL.Path == "/api/settings" ||
		r.URL.Path == "/management/login" ||
		r.URL.Path == "/management" || strings.HasPrefix(r.URL.Path, "/management/") {
		s.mux.ServeHTTP(w, r)
		return
	}
	// Normalise /chat/ → /chat.
	if r.URL.Path == "/chat/" {
		r.URL.Path = "/chat"
	}
	// Chat/API paths: auth enforced at handler registration, skip primary auth.
	// Exception: password mode needs primary auth on chat API routes.
	isConvPath := r.URL.Path == "/api/conversations" || strings.HasPrefix(r.URL.Path, "/api/conversations/")
	isRuntimesPath := r.URL.Path == "/api/runtimes"
	if r.URL.Path == "/chat" || r.URL.Path == "/api/chat" || r.URL.Path == "/api/chat/stream" || r.URL.Path == "/ws/chat" || isConvPath || isRuntimesPath {
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
	fs            fs.FS
	fileServer    http.Handler
	redirectRoot  bool // if true, GET / redirects to /chat
}

func (s *spaFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" {
		// Redirect GET / to /chat if redirectRoot is enabled.
		if s.redirectRoot && r.Method == http.MethodGet {
			http.Redirect(w, r, "/chat", http.StatusFound)
			return
		}
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
	if s.chatSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID := s.resolveUserID(r)
	username, _ := r.Context().Value(ctxUsername).(string)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Query           string       `json:"query"`
		ConversationID  string       `json:"conversation_id,omitempty"`
		NewConversation bool         `json:"new_conversation,omitempty"`
		Attachments     []Attachment `json:"attachments,omitempty"`
		Runtime         string       `json:"runtime,omitempty"`
		Model           string       `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Query == "" && len(req.Attachments) == 0 {
		http.Error(w, "bad request: query or attachments required", http.StatusBadRequest)
		return
	}
	if len(req.Attachments) > 0 {
		var lim AttachmentLimits
		if s.sm != nil {
			lim = EffectiveAttachmentLimits(s.sm.GetEffective().Chat)
		} else {
			lim = EffectiveAttachmentLimits(nil)
		}
		if err := ValidateAttachments(req.Attachments, lim); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Create a new conversation if no conversation_id was supplied.
	conversationID := req.ConversationID
	if conversationID == "" && s.store != nil {
		conversationID = newUUID()
		title := req.Query
		runtime, model := s.resolveNewConversationRuntime(req.Runtime, req.Model)
		if err := s.store.InsertConversation(conversationID, userID, title, runtime, model); err != nil {
			s.logger.Error("store: insert conversation", "err", err)
			conversationID = "" // non-fatal: proceed without conversation tracking
		}
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

	if err := s.chatSessions.StartSession(userID, username, req.Query, req.NewConversation, conversationID, req.Attachments); err != nil {
		if ce, ok := err.(interface{ IsConflict() bool }); ok && ce.IsConflict() {
			http.Error(w, "session already in progress", http.StatusConflict)
			return
		}
		if errors.Is(err, ErrUploadQuotaExceeded) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"user_id": userID, "conversation_id": conversationID})
}

// handleImageFile serves GET /api/images/{convID}/{filename} — agent-produced images.
func (s *Server) handleImageFile(w http.ResponseWriter, r *http.Request) {
	if s.imgStore == nil {
		http.Error(w, "image store not configured", http.StatusServiceUnavailable)
		return
	}
	userID := s.resolveUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	convID := r.PathValue("convID")
	filename := r.PathValue("filename")
	if convID == "" || filename == "" {
		http.NotFound(w, r)
		return
	}
	// Guard against path traversal in the URL segments.
	if strings.Contains(convID, "/") || strings.Contains(convID, "..") ||
		strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	absPath := s.imgStore.AbsPath(convID, filename)
	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	ext := imageFileExt(filename)
	w.Header().Set("Content-Type", extToMime(ext))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filename, time.Time{}, f)
}

// handleChatSSE handles GET /api/chat/stream — streams PTY output and JSON tool events via
// Server-Sent Events. This bypasses transparent HTTP proxies that strip WebSocket Upgrade headers.
// PTY bytes are sent as base64-encoded "pty" events; JSON events as "json" events.
func (s *Server) handleChatSSE(w http.ResponseWriter, r *http.Request) {
	if s.chatSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID := s.resolveUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ptyCh, ptyUnsub, ok := s.chatSessions.SubscribeSession(userID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	defer ptyUnsub()

	jsonCh, jsonUnsub, _ := s.chatSessions.SubscribeJSON(userID)
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
	if s.chatSessions == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}
	userID := s.resolveUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ptyCh, ptyUnsub, ok := s.chatSessions.SubscribeSession(userID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	defer ptyUnsub()

	jsonCh, jsonUnsub, _ := s.chatSessions.SubscribeJSON(userID)
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

// handleManagementWS streams admin session events via WebSocket.
func (s *Server) handleManagementWS(w http.ResponseWriter, r *http.Request) {
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

// handleManagementHistory handles GET /admin/history
func (s *Server) handleManagementHistory(w http.ResponseWriter, r *http.Request) {
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

// handleManagementHistoryDetail handles GET /admin/history/<id>
func (s *Server) handleManagementHistoryDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/management/history/"):]
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

// resolveUserID returns the authenticated user ID from context, or "default" in single-user mode.
func (s *Server) resolveUserID(r *http.Request) string {
	if uid, _ := r.Context().Value(ctxUserID).(string); uid != "" {
		return uid
	}
	return "default"
}

// resolveNewConversationRuntime picks the runtime+model for a freshly created
// conversation. If the caller supplied valid values they win; otherwise the
// server's default runtime is used. Unknown runtimes fall back to the default.
func (s *Server) resolveNewConversationRuntime(runtime, model string) (string, string) {
	if runtime != "" {
		if r, err := agentRuntimeByName(runtime); err == nil {
			if model == "" {
				model = r.DefaultModel
			}
			return r.Name, model
		}
	}
	if s.defaultRuntime.Name != "" {
		m := model
		if m == "" {
			m = s.defaultRuntime.DefaultModel
		}
		return s.defaultRuntime.Name, m
	}
	return "", ""
}

// handleListConversations handles GET /api/conversations with optional cursor
// (?before=<updated_at_ms>) and ?limit=<n>. Response shape:
//
//	{ "pinned": [...], "recent": [...], "next_before": <ms or 0> }
//
// Pinned rows are returned only on the first page (before==0).
func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.store == nil {
		w.Write([]byte(`{"pinned":[],"recent":[]}`))
		return
	}
	userID := s.resolveUserID(r)
	q := r.URL.Query()
	var before int64
	fmt.Sscanf(q.Get("before"), "%d", &before)
	var limit int
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	pinned, recent, nextBefore, err := s.store.ListConversationsPage(userID, before, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if pinned == nil {
		pinned = []ConversationRow{}
	}
	if recent == nil {
		recent = []ConversationRow{}
	}
	resp := map[string]any{
		"pinned":      pinned,
		"recent":      recent,
		"next_before": nextBefore,
	}
	json.NewEncoder(w).Encode(resp)
}

// handlePatchConversation handles PATCH /api/conversations/{id} with body
// {"pinned"?: bool, "runtime"?: string, "model"?: string}. Each field is
// independently applied. On runtime/model change, the corresponding ACP pool
// key is evicted so the next prompt boots a fresh subprocess.
func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	userID := s.resolveUserID(r)

	var req struct {
		Pinned  *bool  `json:"pinned,omitempty"`
		Runtime string `json:"runtime,omitempty"`
		Model   string `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate runtime+model against the registry up front so a bad runtime
	// doesn't leave the row partially updated.
	if req.Runtime != "" {
		rt, err := agentRuntimeByName(req.Runtime)
		if err != nil {
			http.Error(w, "bad request: unknown runtime", http.StatusBadRequest)
			return
		}
		if req.Model != "" {
			ok := false
			for _, m := range rt.Models {
				if m == req.Model {
					ok = true
					break
				}
			}
			if !ok {
				http.Error(w, "bad request: unknown model for runtime", http.StatusBadRequest)
				return
			}
		}
	}
	if req.Model != "" && req.Runtime == "" {
		// Model-only change: validate against the conversation's current runtime.
		conv, err := s.store.GetConversation(id, userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if conv == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		rt, rerr := agentRuntimeByName(conv.Runtime)
		if rerr != nil {
			http.Error(w, "bad request: conversation has unknown runtime; specify runtime too", http.StatusBadRequest)
			return
		}
		ok := false
		for _, m := range rt.Models {
			if m == req.Model {
				ok = true
				break
			}
		}
		if !ok {
			http.Error(w, "bad request: unknown model for runtime", http.StatusBadRequest)
			return
		}
	}

	matched := false
	if req.Pinned != nil {
		ok, err := s.store.UpdateConversationPin(id, userID, *req.Pinned)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		matched = matched || ok
	}
	if req.Runtime != "" || req.Model != "" {
		ok, err := s.store.UpdateConversationRuntime(id, userID, req.Runtime, req.Model)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		matched = matched || ok
		if ok && s.pool != nil {
			s.pool.EvictByKey("chat-api:" + userID + ":" + id)
		}
	}

	// If the request was empty or no fields matched, treat as 404 (the row
	// either doesn't exist or doesn't belong to this user).
	if !matched {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	conv, err := s.store.GetConversation(id, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conv)
}

// handleListConversationMessages handles GET /api/conversations/{id}/messages
// for the authenticated user.
func (s *Server) handleListConversationMessages(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	userID := s.resolveUserID(r)
	conv, err := s.store.GetConversation(id, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	var page, limit int
	fmt.Sscanf(q.Get("page"), "%d", &page)
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	msgs, total, err := s.store.ListMessagesByConversation(id, page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []ConversationMessage{}
	}
	if s.imgStore != nil {
		for i := range msgs {
			msgs[i].Images = s.imgStore.InlineAttachmentsAsDataURIs(msgs[i].Images)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"total": total, "messages": msgs})
}

// handleListRuntimes handles GET /api/runtimes — returns the runtime registry
// for the chat picker. Auth is applied at registration time consistent with
// other /api/* endpoints.
func (s *Server) handleListRuntimes(w http.ResponseWriter, r *http.Request) {
	type runtimeView struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Models       []string `json:"models"`
		DefaultModel string   `json:"default_model"`
		SupportsMCP  bool     `json:"supports_mcp"`
	}
	out := []runtimeView{}
	for _, name := range availableRuntimeNames() {
		rt, err := agentRuntimeByName(name)
		if err != nil {
			continue
		}
		out = append(out, runtimeView{
			ID:           rt.Name,
			Name:         displayRuntimeName(rt.Name),
			Models:       append([]string{}, rt.Models...),
			DefaultModel: rt.DefaultModel,
			SupportsMCP:  rt.SupportsMCP,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"runtimes": out})
}

func displayRuntimeName(id string) string {
	switch id {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	default:
		return id
	}
}

// handleDeleteConversation handles DELETE /api/conversations/{id}.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	userID := s.resolveUserID(r)
	found, err := s.store.DeleteConversation(id, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleManagementListConversations handles
// GET /api/management/conversations with optional ?user, ?q, ?from, ?to,
// ?page, ?limit filters. Read-only; no mutation routes are registered.
func (s *Server) handleManagementListConversations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.store == nil {
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "conversations": []any{}})
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
	rows, total, err := s.store.ListConversationsAdmin(user, keyword, from, to, page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rows == nil {
		rows = []ConversationRow{}
	}
	json.NewEncoder(w).Encode(map[string]any{"total": total, "conversations": rows})
}

// handleManagementGetConversation handles GET /api/management/conversations/{id}.
// Returns the full row regardless of which user owns it.
func (s *Server) handleManagementGetConversation(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	conv, err := s.store.GetConversation(id, "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conv)
}

// handleManagementListConversationMessages handles
// GET /api/management/conversations/{id}/messages.
func (s *Server) handleManagementListConversationMessages(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	var page, limit int
	fmt.Sscanf(q.Get("page"), "%d", &page)
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	if limit <= 0 {
		limit = 50
	}
	msgs, total, err := s.store.ListMessagesByConversation(id, page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []ConversationMessage{}
	}
	if s.imgStore != nil {
		for i := range msgs {
			msgs[i].Images = s.imgStore.InlineAttachmentsAsDataURIs(msgs[i].Images)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"total": total, "messages": msgs})
}

// handleManagementListSchedules handles GET /api/management/schedules with
// optional ?user, ?conv, ?page, ?limit filters.
func (s *Server) handleManagementListSchedules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.store == nil {
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "schedules": []any{}})
		return
	}
	q := r.URL.Query()
	user := q.Get("user")
	conv := q.Get("conv")
	var page, limit int
	fmt.Sscanf(q.Get("page"), "%d", &page)
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	rows, total, err := s.store.ListChatSchedulesAdmin(user, conv, page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rows == nil {
		rows = []ChatSchedule{}
	}
	json.NewEncoder(w).Encode(map[string]any{"total": total, "schedules": rows})
}

// handleManagementAnalytics handles GET /admin/analytics
func (s *Server) handleManagementAnalytics(w http.ResponseWriter, r *http.Request) {
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
