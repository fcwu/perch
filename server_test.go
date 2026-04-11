package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketReceivesPTYOutput(t *testing.T) {
	pm := newPTYManager()
	srv := newServer(pm, nil, nil, nil, nil, nil)

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

func TestSessionsEndpointNoProvider(t *testing.T) {
	pm := newPTYManager()
	s := newServer(pm, nil, nil, nil, nil, nil)
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
	s := newServer(pm, nil, nil, nil, sp, nil)
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
