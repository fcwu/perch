package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type hookHandler struct {
	im *IMManager
}

func newHookHandler(im *IMManager) http.Handler {
	return &hookHandler{im: im}
}

func (h *hookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var event HookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if event.EventName == "Stop" {
		slog.Info("hook Stop event", "json", string(raw))
	}
	if h.im != nil {
		h.im.notify(event)
	}
	w.WriteHeader(http.StatusOK)
}
