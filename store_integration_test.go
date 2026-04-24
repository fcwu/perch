package main

import (
	"encoding/json"
	"testing"
	"time"
)

// TestStoreIntegration verifies that a complete query flow correctly persists
// session + tool events to SQLite.
func TestStoreIntegration(t *testing.T) {
	store := tempStore(t)
	hub := newAdminHub()
	rt := makeTestRuntime()
	m := newUserSessionManager(rt, "", nil, store, hub)

	// Start a session
	if err := m.StartSession("u1", "alice", "what is HBS?", false, ""); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Claim a UUID (simulates first hook arriving from OpenCode)
	sess, ok := m.ClaimUUID("uuid-sess-1")
	if !ok || sess == nil {
		t.Fatal("ClaimUUID failed")
	}

	// Verify session was inserted in store
	time.Sleep(10 * time.Millisecond)
	detail, err := store.GetSession("uuid-sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil {
		t.Fatal("expected session in store after ClaimUUID")
	}
	if detail.Status != "running" {
		t.Errorf("expected status='running', got %q", detail.Status)
	}

	// Simulate PreToolUse hook
	inputJSON, _ := json.Marshal(map[string]string{"path": "wiki/hbs.md"})
	m.NotifyHook(HookEvent{
		EventName: "PreToolUse",
		SessionID: "uuid-sess-1",
		ToolName:  "Read",
		ToolInput: inputJSON,
	})

	// Verify tool event was inserted
	time.Sleep(10 * time.Millisecond)
	detail, err = store.GetSession("uuid-sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ToolEvents) != 1 {
		t.Fatalf("expected 1 tool event, got %d", len(detail.ToolEvents))
	}
	if detail.ToolEvents[0].ToolName != "Read" {
		t.Errorf("expected tool_name='Read', got %q", detail.ToolEvents[0].ToolName)
	}

	// Simulate PostToolUse hook
	outputJSON, _ := json.Marshal("HBS content here")
	m.NotifyHook(HookEvent{
		EventName:    "PostToolUse",
		SessionID:    "uuid-sess-1",
		ToolName:     "Read",
		ToolResponse: outputJSON,
	})

	// Simulate Stop hook
	m.NotifyHook(HookEvent{
		EventName:            "Stop",
		SessionID:            "uuid-sess-1",
		LastAssistantMessage: "HBS is a backup solution.",
	})

	// Verify session is marked done
	time.Sleep(10 * time.Millisecond)
	detail, err = store.GetSession("uuid-sess-1")
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
