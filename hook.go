package main

import (
	"encoding/json"
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
	var event HookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if h.im != nil {
		h.im.notify(event)
	}
	w.WriteHeader(http.StatusOK)
}
