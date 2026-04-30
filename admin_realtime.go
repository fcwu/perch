package main

import (
	"encoding/json"
	"sync"
)

// ManagementSessionView is the lightweight view pushed over the WebSocket.
type ManagementSessionView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Query       string `json:"query"`
	Status      string `json:"status"`
	CurrentTool string `json:"current_tool,omitempty"`
	StartedAt   int64  `json:"started_at"`
}

type managementEvent struct {
	Type    string            `json:"type"`
	Session *ManagementSessionView `json:"session,omitempty"`
	ID      string            `json:"id,omitempty"`
	Status  string            `json:"status,omitempty"`
}

// ManagementHub broadcasts delta session events to all connected admin WebSocket clients.
type ManagementHub struct {
	mu       sync.Mutex
	clients  map[chan string]struct{}
	sessions map[string]*ManagementSessionView // sessionID → view
}

func newManagementHub() *ManagementHub {
	return &ManagementHub{
		clients:  make(map[chan string]struct{}),
		sessions: make(map[string]*ManagementSessionView),
	}
}

// subscribe registers a new admin client and returns a snapshot + event channel.
func (h *ManagementHub) subscribe() (<-chan string, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan string, 64)
	h.clients[ch] = struct{}{}

	// Send initial snapshot
	sessions := make([]*ManagementSessionView, 0, len(h.sessions))
	for _, s := range h.sessions {
		v := *s
		sessions = append(sessions, &v)
	}
	snap, _ := json.Marshal(map[string]any{"type": "session_snapshot", "sessions": sessions})
	ch <- string(snap)

	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
		close(ch)
	}
}

func (h *ManagementHub) broadcast(ev managementEvent) {
	data, _ := json.Marshal(ev)
	msg := string(data)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// SessionAdded notifies all clients that a new session started.
func (h *ManagementHub) SessionAdded(v ManagementSessionView) {
	h.mu.Lock()
	cp := v
	h.sessions[v.ID] = &cp
	h.mu.Unlock()
	h.broadcast(managementEvent{Type: "session_added", Session: &v})
}

// SessionUpdated notifies all clients of a tool state change.
func (h *ManagementHub) SessionUpdated(sessionID, currentTool string) {
	h.mu.Lock()
	if s, ok := h.sessions[sessionID]; ok {
		s.CurrentTool = currentTool
	}
	h.mu.Unlock()
	h.broadcast(managementEvent{Type: "session_update", ID: sessionID, Session: &ManagementSessionView{ID: sessionID, CurrentTool: currentTool}})
}

// SessionRemoved notifies all clients that a session ended.
func (h *ManagementHub) SessionRemoved(sessionID, status string) {
	h.mu.Lock()
	delete(h.sessions, sessionID)
	h.mu.Unlock()
	h.broadcast(managementEvent{Type: "session_removed", ID: sessionID, Status: status})
}
