package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketReceivesPTYOutput(t *testing.T) {
	pm := newPTYManager()
	srv := newServer(pm, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		pm.broadcast([]byte("hello from pty"))
	}()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if string(msg) != "hello from pty" {
		t.Errorf("expected 'hello from pty', got %q", string(msg))
	}
}

type fakeSessionProvider struct {
	sessions []SessionView
}

func (f *fakeSessionProvider) ListSessions() []SessionView { return f.sessions }
func (f *fakeSessionProvider) SubscribeSession(channelID string) (<-chan []byte, func(), bool) {
	return nil, nil, false
}
func (f *fakeSessionProvider) ResizeSession(_ string, _, _ uint16) {}
func (f *fakeSessionProvider) WriteSession(_ string, _ []byte) error { return nil }

func TestSessionsEndpointNoProvider(t *testing.T) {
	pm := newPTYManager()
	s := newServer(pm, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "[]" {
		t.Fatalf("expected empty JSON array, got %q", body)
	}
}

func TestSessionsEndpointWithProvider(t *testing.T) {
	pm := newPTYManager()
	sp := &fakeSessionProvider{sessions: []SessionView{{ChannelID: "ch1", SessionUUID: "uuid1"}}}
	s := newServer(pm, nil, nil, sp, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ch1") {
		t.Fatalf("response should contain channel id, got %q", rr.Body.String())
	}
}

type recordingSessionProvider struct {
	ch       chan []byte
	written  [][]byte
	resized  [][2]uint16
}

func newRecordingSessionProvider() *recordingSessionProvider {
	return &recordingSessionProvider{ch: make(chan []byte, 4)}
}

func (r *recordingSessionProvider) ListSessions() []SessionView { return nil }
func (r *recordingSessionProvider) SubscribeSession(_ string) (<-chan []byte, func(), bool) {
	return r.ch, func() {}, true
}
func (r *recordingSessionProvider) ResizeSession(_ string, cols, rows uint16) {
	r.resized = append(r.resized, [2]uint16{cols, rows})
}
func (r *recordingSessionProvider) WriteSession(_ string, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	r.written = append(r.written, cp)
	return nil
}

func TestSessionWSKeystrokeForwarded(t *testing.T) {
	pm := newPTYManager()
	sp := newRecordingSessionProvider()
	srv := newServer(pm, nil, nil, sp, nil, nil, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/session?id=ch1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("hello")); err != nil {
		t.Fatalf("ws write: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if len(sp.written) == 0 || string(sp.written[0]) != "hello" {
		t.Fatalf("expected keystroke forwarded, got %v", sp.written)
	}
}

func postChat(t *testing.T, s *Server, userID, query string, newConversation bool) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query, "new_conversation": newConversation})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), ctxUserID, userID)
	ctx = context.WithValue(ctx, ctxUsername, "testuser")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleChatAPI(rr, req)
	return rr
}

func TestHandleChatAPIHistory(t *testing.T) {
	store := tempStore(t)
	rt := makeTestRuntime()
	hub := newAdminHub()
	m := newUserSessionManager(rt, "", nil, store, hub)

	// Insert a prior completed session for userID "u1"
	store.InsertSession("prior-sess", "u1", "testuser", "prior query")
	store.UpdateSessionDone("prior-sess", "prior response")

	srv := newServer(newPTYManager(), nil, nil, nil, m, nil, nil, nil, nil, nil, nil)

	// Second request from same user within 24h should use history
	rr := postChat(t, srv, "u1", "follow-up question", false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the session's prompt includes conversation history
	m.mu.Lock()
	sess := m.sessions["u1"]
	m.mu.Unlock()
	if sess == nil {
		t.Fatal("expected session to exist")
	}
	if !strings.Contains(sess.prompt, "<conversation_history>") {
		t.Errorf("expected prompt to include conversation history, got:\n%s", sess.prompt)
	}
	if !strings.Contains(sess.prompt, "prior query") {
		t.Errorf("expected prompt to include prior query, got:\n%s", sess.prompt)
	}
}

func TestHandleChatAPINewConversation(t *testing.T) {
	store := tempStore(t)
	rt := makeTestRuntime()
	hub := newAdminHub()
	m := newUserSessionManager(rt, "", nil, store, hub)

	// Insert a prior completed session
	store.InsertSession("prior-sess2", "u2", "testuser", "old query")
	store.UpdateSessionDone("prior-sess2", "old response")

	srv := newServer(newPTYManager(), nil, nil, nil, m, nil, nil, nil, nil, nil, nil)

	// Request with new_conversation:true should skip history
	rr := postChat(t, srv, "u2", "fresh start", true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	m.mu.Lock()
	sess := m.sessions["u2"]
	m.mu.Unlock()
	if sess == nil {
		t.Fatal("expected session to exist")
	}
	if strings.Contains(sess.prompt, "<conversation_history>") {
		t.Errorf("expected no conversation history when new_conversation=true, got:\n%s", sess.prompt)
	}
	if sess.prompt != "fresh start" {
		t.Errorf("expected raw query as prompt, got: %q", sess.prompt)
	}
}

func TestSessionWSResizeNotForwarded(t *testing.T) {
	pm := newPTYManager()
	sp := newRecordingSessionProvider()
	srv := newServer(pm, nil, nil, sp, nil, nil, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/session?id=ch1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	msg := `{"type":"resize","cols":120,"rows":40}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("ws write: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if len(sp.written) != 0 {
		t.Fatalf("resize should not be forwarded to WriteSession, got %v", sp.written)
	}
	if len(sp.resized) == 0 {
		t.Fatal("ResizeSession should have been called")
	}
}


func TestPasswordModeGitLabRouteReturns404(t *testing.T) {
	pm := newPTYManager()
	auth := newAuthMiddleware("password", "secret")
	ga := &gitLabAuth{
		clientID:     "id",
		clientSecret: "secret",
		gitlabURL:    "http://gitlab.example.com",
		authMethod:   "gitlab",
		mode:         ModeSingle,
		adminIDs:     map[string]bool{},
		allowedIDs:   map[string]bool{},
	}
	srv := newServerWithMode(pm, auth, nil, nil, nil, ga, nil, nil, nil, nil, nil, ModeSingle, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/gitlab", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("/auth/gitlab in password mode: expected 404, got %d", rr.Code)
	}
}
