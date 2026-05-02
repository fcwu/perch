package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestMCPServerInitialize covers the in-process protocol path: initialize and
// tools/list should produce well-formed JSON-RPC responses without touching
// the database.
func TestMCPServerInitialize(t *testing.T) {
	store := openTestStore(t)
	srv := &mcpServer{store: store, userID: "user-a", convID: "conv-a"}

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n")
	var out bytes.Buffer
	srv.run(in, &out)

	scanner := bufio.NewScanner(&out)
	var responses []map[string]any
	for scanner.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatalf("non-JSON line: %s err=%v", scanner.Text(), err)
		}
		responses = append(responses, m)
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d: %v", len(responses), responses)
	}
	// initialize advertises the three tools schema in tools/list response.
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}
	wantNames := map[string]bool{"schedule_message": false, "list_schedules": false, "cancel_schedule": false}
	for _, ti := range tools {
		name := ti.(map[string]any)["name"].(string)
		wantNames[name] = true
		// Ensure no tool exposes user_id or conversation_id parameters.
		schema, _ := ti.(map[string]any)["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		for k := range props {
			if k == "user_id" || k == "conversation_id" {
				t.Errorf("tool %s leaks identity field %q", name, k)
			}
		}
	}
	for n, found := range wantNames {
		if !found {
			t.Errorf("missing tool: %s", n)
		}
	}
}

// TestMCPScheduleMessageBindsIdentity verifies that schedule_message ignores
// any user_id / conversation_id fields supplied in arguments and uses the
// env-bound identity instead.
func TestMCPScheduleMessageBindsIdentity(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("conv-a", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	srv := &mcpServer{store: store, userID: "user-a", convID: "conv-a"}

	hourMs := time.Now().UnixMilli() + 3600_000
	args := map[string]any{
		"prompt":          "ping",
		"one_shot_at":     hourMs,
		"user_id":         "evil",      // attempt to override
		"conversation_id": "other-conv", // attempt to override
	}
	argsJSON, _ := json.Marshal(args)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "schedule_message",
			"arguments": json.RawMessage(argsJSON),
		},
	}
	reqJSON, _ := json.Marshal(req)
	in := bytes.NewReader(append(reqJSON, '\n'))
	var out bytes.Buffer
	srv.run(in, &out)

	rows, _ := store.LoadAllChatSchedules()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row inserted, got %d", len(rows))
	}
	if rows[0].UserID != "user-a" || rows[0].ConversationID != "conv-a" {
		t.Errorf("identity not bound from env: got user=%s conv=%s", rows[0].UserID, rows[0].ConversationID)
	}
}

// TestMCPCancelScheduleScopedToIdentity confirms cancel_schedule won't delete
// rows owned by a different (user, conv).
func TestMCPCancelScheduleScopedToIdentity(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("conv-a", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertConversation("conv-b", "user-b", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	hour, minute := 9, 0
	if err := store.InsertChatSchedule(&ChatSchedule{
		ID: "row-b", UserID: "user-b", ConversationID: "conv-b",
		Hour: &hour, Minute: &minute, Repeat: true, Prompt: "x", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	srv := &mcpServer{store: store, userID: "user-a", convID: "conv-a"}
	req := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "cancel_schedule",
			"arguments": json.RawMessage(`{"id":"row-b"}`),
		},
	}
	reqJSON, _ := json.Marshal(req)
	var out bytes.Buffer
	srv.run(bytes.NewReader(append(reqJSON, '\n')), &out)

	// Row should still exist — different user can't cancel it.
	rows, _ := store.LoadAllChatSchedules()
	if len(rows) != 1 {
		t.Errorf("foreign cancel deleted row(s); rows=%d", len(rows))
	}
}
