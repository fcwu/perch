package main

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	pty    *PTYManager
	auth   *AuthMiddleware
	sched  *Scheduler
	logger *slog.Logger
	mux    *http.ServeMux
}

func newServer(pm *PTYManager, auth *AuthMiddleware, sched *Scheduler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{pty: pm, auth: auth, sched: sched, logger: logger, mux: http.NewServeMux()}
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/input", s.handleInput)
	if auth != nil && auth.mode == "password" {
		s.mux.HandleFunc("/login", auth.handleLogin)
	}
	if sched != nil {
		s.mux.Handle("/schedule", sched)
		s.mux.Handle("/schedule/", sched)
	}
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err == nil {
		s.mux.Handle("/", http.FileServer(http.FS(distFS)))
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		s.auth.wrap(s.mux).ServeHTTP(w, r)
	} else {
		s.mux.ServeHTTP(w, r)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
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
