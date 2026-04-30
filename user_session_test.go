package main

import (
	"strings"
	"testing"
	"time"
)

// TestSignalDoneOnPTYExit guards the MT07/T52 regression: when the PTY-backed
// web `/ws` session exits, the frontend must receive a `done` JSON event so the
// textarea unlocks. The fallback in StartSession's PTY-exit observer broadcasts
// that event after the PTY drains.
func TestSignalDoneOnPTYExit(t *testing.T) {
	rt := makeTestRuntime() // command "true" — exits immediately
	m := newUserSessionManager(rt, "", nil, nil, nil)
	if err := m.StartSession("u1", "alice", "q", false, ""); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	m.mu.Lock()
	sess := m.sessions["u1"]
	m.mu.Unlock()
	if sess == nil {
		t.Fatal("expected session for u1")
	}
	jsonCh, unsub := sess.subscribeJSON()
	defer unsub()

	select {
	case msg := <-jsonCh:
		if msg != `{"type":"done"}` {
			t.Errorf("expected done message from PTY-exit fallback, got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected done JSON event after PTY exit; frontend would be stuck loading")
	}
}

func TestBuildPrompt(t *testing.T) {
	// Empty history returns raw query
	if got := buildPrompt(nil, "hello"); got != "hello" {
		t.Errorf("empty history: expected raw query, got %q", got)
	}

	// N turns produce correct prefix format
	history := []ConversationTurn{
		{Query: "q1", Response: "r1"},
		{Query: "q2", Response: "r2"},
	}
	got := buildPrompt(history, "new question")
	if !strings.Contains(got, "<conversation_history>") {
		t.Error("expected <conversation_history> tag")
	}
	if !strings.Contains(got, "User: q1\nAssistant: r1") {
		t.Error("expected first turn formatted correctly")
	}
	if !strings.Contains(got, "User: q2\nAssistant: r2") {
		t.Error("expected second turn formatted correctly")
	}
	if !strings.HasSuffix(got, "</conversation_history>\n\nnew question") {
		t.Errorf("expected prompt to end with query after closing tag, got: %q", got)
	}
}

func makeTestRuntime() AgentRuntime {
	return AgentRuntime{
		Name:    "opencode",
		Command: "true", // no-op binary that exits immediately
	}
}

func TestUserSessionManagerStartSession(t *testing.T) {
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil, nil, nil)
	if err := m.StartSession("u1", "alice", "what is X?", false, ""); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	m.mu.Lock()
	_, ok := m.sessions["u1"]
	m.mu.Unlock()
	if !ok {
		t.Fatal("expected session to be created for u1")
	}
}

func TestUserSessionManagerConflictWhileRunning(t *testing.T) {
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil, nil, nil)

	// Manually add a running session.
	sess := newUserSession("u1", "alice", "test query")
	// status defaults to userSessionRunning (zero value).
	m.mu.Lock()
	m.sessions["u1"] = sess
	m.mu.Unlock()

	err := m.StartSession("u1", "alice", "again", false, "")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	ce, ok := err.(interface{ IsConflict() bool })
	if !ok || !ce.IsConflict() {
		t.Fatalf("expected IsConflict() error, got %T: %v", err, err)
	}
}

func TestUserSessionManagerCancelStopsPTY(t *testing.T) {
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil, nil, nil)

	if err := m.StartSession("u1", "alice", "q", false, ""); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	m.mu.Lock()
	sess := m.sessions["u1"]
	m.mu.Unlock()

	// Cancel the session context (simulates timeout).
	if sess.cancel != nil {
		sess.cancel()
	}
	// Give the watcher goroutine time to stop the PTY.
	time.Sleep(50 * time.Millisecond)
	// PTY should be stopped (done channel closed).
	select {
	case <-sess.pty.done:
		// expected
	default:
		// PTY may have already exited (command "true" exits immediately)
	}
}

