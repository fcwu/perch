package main

import (
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndListConversations(t *testing.T) {
	s := openTestStore(t)

	if err := s.InsertConversation("conv-1", "user-a", "Hello world, this is my first message", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}
	if err := s.InsertConversation("conv-2", "user-a", "Second conversation", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}
	// Different user — should not appear in user-a's list.
	if err := s.InsertConversation("conv-3", "user-b", "Other user", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}

	convs, err := s.ListConversations("user-a")
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convs))
	}
	// InsertConversation with long title should be truncated to ≤60 runes.
	longTitle := "This is a very long title that definitely exceeds sixty characters in total"
	if err := s.InsertConversation("conv-long", "user-a", longTitle, "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}
	convs, err = s.ListConversations("user-a")
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	for _, c := range convs {
		if c.ID == "conv-long" {
			if len([]rune(c.Title)) > 60 {
				t.Errorf("title not truncated: %q (len=%d)", c.Title, len([]rune(c.Title)))
			}
		}
	}
}

func TestDeleteConversation(t *testing.T) {
	s := openTestStore(t)

	if err := s.InsertConversation("del-conv", "user-x", "To be deleted", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}

	// Delete by wrong user → not found.
	found, err := s.DeleteConversation("del-conv", "user-y")
	if err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if found {
		t.Error("expected not found when user_id doesn't match")
	}

	// Delete by correct user → found.
	found, err = s.DeleteConversation("del-conv", "user-x")
	if err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if !found {
		t.Error("expected found=true")
	}

	// Second delete → not found.
	found, err = s.DeleteConversation("del-conv", "user-x")
	if err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if found {
		t.Error("expected found=false on second delete")
	}
}

func TestDeleteConversationCleansUpSessions(t *testing.T) {
	s := openTestStore(t)

	if err := s.InsertConversation("c1", "user-z", "Conversation with sessions", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}
	// Insert a session belonging to this conversation.
	if err := s.InsertSessionWithConversation("sess-1", "user-z", "user-z", "query", "c1"); err != nil {
		t.Fatalf("InsertSessionWithConversation: %v", err)
	}

	found, err := s.DeleteConversation("c1", "user-z")
	if err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if !found {
		t.Error("expected found=true")
	}

	// The session should be gone.
	detail, err := s.GetSession("sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if detail != nil {
		t.Error("expected session to be deleted with conversation")
	}
}

func TestTouchConversation(t *testing.T) {
	s := openTestStore(t)

	if err := s.InsertConversation("touch-conv", "user-t", "Touch test", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("InsertConversation: %v", err)
	}

	convs, err := s.ListConversations("user-t")
	if err != nil || len(convs) == 0 {
		t.Fatalf("ListConversations: %v, len=%d", err, len(convs))
	}
	originalUpdatedAt := convs[0].UpdatedAt

	time.Sleep(5 * time.Millisecond) // ensure clock advances
	if err := s.TouchConversation("touch-conv"); err != nil {
		t.Fatalf("TouchConversation: %v", err)
	}

	convs, err = s.ListConversations("user-t")
	if err != nil || len(convs) == 0 {
		t.Fatalf("ListConversations: %v", err)
	}
	if convs[0].UpdatedAt <= originalUpdatedAt {
		t.Errorf("expected updated_at to increase: before=%d after=%d", originalUpdatedAt, convs[0].UpdatedAt)
	}
}
