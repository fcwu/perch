package main

import (
	"testing"
	"time"
)

// TestStoreIntegration verifies that a complete query flow correctly persists
// session + tool events to SQLite via direct store calls (ACP-driven path).
func TestStoreIntegration(t *testing.T) {
	store := tempStore(t)

	// Insert session (as ACPUserSessionManager does at prompt start)
	if err := store.InsertSession("sess-1", "u1", "alice", "what is HBS?"); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// Verify session was inserted
	detail, err := store.GetSession("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil {
		t.Fatal("expected session in store after InsertSession")
	}
	if detail.Status != "running" {
		t.Errorf("expected status='running', got %q", detail.Status)
	}

	// Insert a tool event (as ACPUserSessionManager does on tool_call_started)
	eventID, err := store.InsertToolEvent("sess-1", "Read", `{"path":"wiki/hbs.md"}`)
	if err != nil {
		t.Fatalf("InsertToolEvent: %v", err)
	}
	if eventID == 0 {
		t.Fatal("expected non-zero tool event ID")
	}

	// Verify tool event was inserted
	detail, err = store.GetSession("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ToolEvents) != 1 {
		t.Fatalf("expected 1 tool event, got %d", len(detail.ToolEvents))
	}
	if detail.ToolEvents[0].ToolName != "Read" {
		t.Errorf("expected tool_name='Read', got %q", detail.ToolEvents[0].ToolName)
	}

	// Update tool event end (as ACPUserSessionManager does on tool_call_completed)
	if err := store.UpdateToolEventEnd(eventID, `{"command":"cat file.txt"}`, `"HBS content here"`); err != nil {
		t.Fatalf("UpdateToolEventEnd: %v", err)
	}

	// Mark session done (as ACPUserSessionManager does on RunCompleted)
	if err := store.UpdateSessionDone("sess-1", "HBS is a backup solution."); err != nil {
		t.Fatalf("UpdateSessionDone: %v", err)
	}

	// Allow any async writes to settle
	time.Sleep(5 * time.Millisecond)

	// Verify session is marked done
	detail, err = store.GetSession("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "done" {
		t.Errorf("expected status='done', got %q", detail.Status)
	}
	if detail.Response == nil || *detail.Response != "HBS is a backup solution." {
		t.Errorf("unexpected response: %v", detail.Response)
	}
}
