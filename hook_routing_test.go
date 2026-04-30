package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestUserSessionManager() *UserSessionManager {
	rt := AgentRuntime{Name: "opencode", Command: "true"}
	return newUserSessionManager(rt, "", nil, nil, nil)
}

func TestHookRouterKnownSessionUUID(t *testing.T) {
	usm := newTestUserSessionManager()
	// Seed a session and claim a UUID.
	usm.sessions["u1"] = newUserSession("u1", "alice", "test query")
	usm.sessions["u1"].sessionUUID = "uuid-123"
	usm.uuidMap["uuid-123"] = "u1"

	jsonCh, unsub := usm.sessions["u1"].subscribeJSON()
	defer unsub()

	h := newHookHandler(nil, usm)
	body := `{"hook_event_name":"PreToolUse","session_id":"uuid-123","tool_name":"Read"}`
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	select {
	case msg := <-jsonCh:
		if len(msg) == 0 {
			t.Fatal("expected non-empty JSON message")
		}
	default:
		t.Fatal("expected a JSON message to be broadcast")
	}
}

func TestHookRouterUnknownSessionUUIDDiscarded(t *testing.T) {
	usm := newTestUserSessionManager()
	// No sessions registered.
	h := newHookHandler(nil, usm)
	body := `{"hook_event_name":"PreToolUse","session_id":"uuid-unknown","tool_name":"Bash"}`
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	// Should still return 200 — unknown session is just discarded.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHookRouterStopMarksSessionCompleted(t *testing.T) {
	usm := newTestUserSessionManager()
	sess := newUserSession("u1", "alice", "test query")
	sess.sessionUUID = "uuid-stop"
	usm.sessions["u1"] = sess
	usm.uuidMap["uuid-stop"] = "u1"

	jsonCh, unsub := sess.subscribeJSON()
	defer unsub()

	h := newHookHandler(nil, usm)
	body := `{"hook_event_name":"Stop","session_id":"uuid-stop"}`
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Stop hook must NOT broadcast `done` — the PTY-exit goroutine does
	// that after the buffer drains, otherwise trailing assistant bytes get
	// cut off before the SSE client renders them.
	select {
	case msg := <-jsonCh:
		t.Fatalf("Stop hook unexpectedly broadcast: %q", msg)
	default:
	}

	sess.mu.Lock()
	st := sess.status
	sess.mu.Unlock()
	if st != userSessionCompleted {
		t.Errorf("expected session to be completed, got %d", st)
	}
}
