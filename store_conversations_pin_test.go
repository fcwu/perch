package main

import (
	"testing"
)

func TestUpdateConversationPin(t *testing.T) {
	s := openTestStore(t)
	if err := s.InsertConversation("c1", "user-a", "Test", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}

	ok, err := s.UpdateConversationPin("c1", "user-a", true)
	if err != nil || !ok {
		t.Fatalf("UpdateConversationPin: ok=%v err=%v", ok, err)
	}
	conv, err := s.GetConversation("c1", "user-a")
	if err != nil || conv == nil {
		t.Fatalf("GetConversation: %v %v", conv, err)
	}
	if !conv.Pinned || conv.PinnedAt == nil {
		t.Errorf("expected pinned=true with pinned_at set, got pinned=%v pinned_at=%v", conv.Pinned, conv.PinnedAt)
	}

	// Wrong user — silent no-op.
	ok, err = s.UpdateConversationPin("c1", "other-user", true)
	if err != nil {
		t.Fatalf("UpdateConversationPin (other user): %v", err)
	}
	if ok {
		t.Error("expected ok=false when user does not match")
	}

	// Unpin clears pinned_at.
	ok, err = s.UpdateConversationPin("c1", "user-a", false)
	if err != nil || !ok {
		t.Fatalf("Unpin: ok=%v err=%v", ok, err)
	}
	conv, _ = s.GetConversation("c1", "user-a")
	if conv.Pinned || conv.PinnedAt != nil {
		t.Errorf("expected pinned=false with nil pinned_at, got %+v", conv)
	}
}

func TestUpdateConversationRuntime(t *testing.T) {
	s := openTestStore(t)
	if err := s.InsertConversation("c1", "user-a", "Test", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}

	ok, err := s.UpdateConversationRuntime("c1", "user-a", "opencode", "opencode-default")
	if err != nil || !ok {
		t.Fatalf("UpdateConversationRuntime: ok=%v err=%v", ok, err)
	}
	conv, _ := s.GetConversation("c1", "user-a")
	if conv.Runtime != "opencode" || conv.Model != "opencode-default" {
		t.Errorf("expected opencode/opencode-default, got %s/%s", conv.Runtime, conv.Model)
	}

	// Cross-user write — no-op.
	ok, err = s.UpdateConversationRuntime("c1", "other-user", "claude", "claude-opus-4-7")
	if err != nil {
		t.Fatalf("UpdateConversationRuntime: %v", err)
	}
	if ok {
		t.Error("expected ok=false on cross-user write")
	}
	conv, _ = s.GetConversation("c1", "user-a")
	if conv.Runtime != "opencode" {
		t.Errorf("conversation runtime corrupted by cross-user write: %s", conv.Runtime)
	}
}

func TestListConversationsPagePinnedAndRecent(t *testing.T) {
	s := openTestStore(t)
	for i, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
		if err := s.InsertConversation(id, "user-a", id, "claude", "claude-sonnet-4-6"); err != nil {
			t.Fatalf("InsertConversation %s: %v", id, err)
		}
		// Force monotonically distinct updated_at values.
		_ = i
	}
	// Pin two of them — expect them grouped on the first page.
	if _, err := s.UpdateConversationPin("c2", "user-a", true); err != nil {
		t.Fatalf("UpdateConversationPin: %v", err)
	}
	if _, err := s.UpdateConversationPin("c4", "user-a", true); err != nil {
		t.Fatalf("UpdateConversationPin: %v", err)
	}

	pinned, recent, nextBefore, err := s.ListConversationsPage("user-a", 0, 10)
	if err != nil {
		t.Fatalf("ListConversationsPage: %v", err)
	}
	if len(pinned) != 2 {
		t.Errorf("expected 2 pinned, got %d", len(pinned))
	}
	if len(recent) != 3 {
		t.Errorf("expected 3 recent, got %d", len(recent))
	}
	if nextBefore != 0 {
		t.Errorf("expected nextBefore=0 (last page), got %d", nextBefore)
	}
	// Subsequent page with cursor should return only non-pinned rows.
	_, recent2, _, err := s.ListConversationsPage("user-a", 1<<60, 2)
	if err != nil {
		t.Fatalf("ListConversationsPage: %v", err)
	}
	if len(recent2) != 2 {
		t.Errorf("expected 2 rows on cursor page, got %d", len(recent2))
	}
}

func TestDeleteConversationCascadesSchedules(t *testing.T) {
	s := openTestStore(t)
	if err := s.InsertConversation("c1", "user-a", "Test", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}
	hour, minute := 9, 0
	sch := &ChatSchedule{
		UserID:         "user-a",
		ConversationID: "c1",
		Hour:           &hour,
		Minute:         &minute,
		Repeat:         true,
		Prompt:         "ping",
		Enabled:        true,
	}
	if err := s.InsertChatSchedule(sch); err != nil {
		t.Fatalf("InsertChatSchedule: %v", err)
	}
	found, err := s.DeleteConversation("c1", "user-a")
	if err != nil || !found {
		t.Fatalf("DeleteConversation: %v %v", found, err)
	}
	rows, _ := s.ListChatSchedules("user-a", "c1")
	if len(rows) != 0 {
		t.Errorf("expected schedules cascaded, got %d rows", len(rows))
	}
}

func TestChatScheduleValidation(t *testing.T) {
	now := int64(1_000_000)
	hour, minute := 9, 0
	cases := []struct {
		name      string
		prompt    string
		hour      *int
		minute    *int
		oneShot   int64
		expectErr bool
	}{
		{"daily-ok", "ping", &hour, &minute, 0, false},
		{"oneshot-ok", "ping", nil, nil, now + 60_000, false},
		{"both-set", "ping", &hour, &minute, now + 60_000, true},
		{"none-set", "ping", nil, nil, 0, true},
		{"empty-prompt", "", &hour, &minute, 0, true},
		{"oneshot-past", "ping", nil, nil, now - 1, true},
	}
	for _, c := range cases {
		err := validateChatScheduleInput(c.prompt, c.hour, c.minute, c.oneShot, now)
		gotErr := err != nil
		if gotErr != c.expectErr {
			t.Errorf("%s: expectErr=%v got err=%v", c.name, c.expectErr, err)
		}
	}
}
