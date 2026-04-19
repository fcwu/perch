package main

import (
	"encoding/json"
	"net/http"
)

type hookHandler struct {
	im           *IMManager
	userSessions *UserSessionManager
}

func newHookHandler(im *IMManager, userSessions *UserSessionManager) http.Handler {
	return &hookHandler{im: im, userSessions: userSessions}
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
	if h.im != nil {
		h.im.notify(event)
	}
	if h.userSessions != nil {
		h.userSessions.NotifyHook(event)
	}
	w.WriteHeader(http.StatusOK)
}
