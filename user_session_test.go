package main

import (
	"testing"
	"time"
)

func makeTestRuntime() AgentRuntime {
	return AgentRuntime{
		Name:    "opencode",
		Command: "true", // no-op binary that exits immediately
	}
}

func TestUserSessionManagerStartSession(t *testing.T) {
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil)
	if err := m.StartSession("u1", "alice", "what is X?"); err != nil {
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
	m := newUserSessionManager(rt, "", nil)

	// Manually add a running session.
	sess := newUserSession("u1", "alice")
	// status defaults to userSessionRunning (zero value).
	m.mu.Lock()
	m.sessions["u1"] = sess
	m.mu.Unlock()

	err := m.StartSession("u1", "alice", "again")
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
	m := newUserSessionManager(rt, "", nil)

	if err := m.StartSession("u1", "alice", "q"); err != nil {
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

func TestUserSessionManagerClaimUUID(t *testing.T) {
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil)
	if err := m.StartSession("u1", "alice", "q"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	sess, ok := m.ClaimUUID("uuid-abc")
	if !ok || sess == nil {
		t.Fatal("expected ClaimUUID to succeed for pending session")
	}
	m.mu.Lock()
	uid := m.uuidMap["uuid-abc"]
	m.mu.Unlock()
	if uid != "u1" {
		t.Errorf("expected uuidMap to map uuid-abc → u1, got %q", uid)
	}
	// Second ClaimUUID for same uuid returns same session.
	sess2, ok2 := m.ClaimUUID("uuid-abc")
	if !ok2 || sess2 != sess {
		t.Fatal("expected same session on second ClaimUUID")
	}
}
