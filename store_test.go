package main

import (
	"os"
	"sync"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "perch-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	s, err := OpenStore(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreSessionLifecycle(t *testing.T) {
	s := tempStore(t)

	if err := s.InsertSession("sess1", "u1", "alice", "what is HBS?"); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// Verify session is running
	detail, err := s.GetSession("sess1")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil {
		t.Fatal("expected session, got nil")
	}
	if detail.Status != "running" {
		t.Errorf("expected status='running', got %q", detail.Status)
	}

	// Insert tool events
	eventID, err := s.InsertToolEvent("sess1", "read", `{"path":"wiki/hbs.md"}`)
	if err != nil {
		t.Fatalf("InsertToolEvent: %v", err)
	}
	if eventID <= 0 {
		t.Errorf("expected positive event ID, got %d", eventID)
	}

	if err := s.UpdateToolEventEnd(eventID, `{"command":"ls"}`, `{"result":"ok"}`); err != nil {
		t.Fatalf("UpdateToolEventEnd: %v", err)
	}

	// Complete session
	if err := s.UpdateSessionDone("sess1", "HBS is a backup solution."); err != nil {
		t.Fatalf("UpdateSessionDone: %v", err)
	}

	detail, err = s.GetSession("sess1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "done" {
		t.Errorf("expected status='done', got %q", detail.Status)
	}
	if detail.Response == nil || *detail.Response != "HBS is a backup solution." {
		t.Errorf("unexpected response: %v", detail.Response)
	}
	if len(detail.ToolEvents) != 1 {
		t.Errorf("expected 1 tool event, got %d", len(detail.ToolEvents))
	}
}

func TestStoreSessionTimeout(t *testing.T) {
	s := tempStore(t)

	if err := s.InsertSession("sess2", "u2", "bob", "query"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSessionTimeout("sess2"); err != nil {
		t.Fatalf("UpdateSessionTimeout: %v", err)
	}
	detail, err := s.GetSession("sess2")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "timeout" {
		t.Errorf("expected status='timeout', got %q", detail.Status)
	}
}

func TestStoreConcurrentWrites(t *testing.T) {
	s := tempStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "sess-concurrent-" + string(rune('A'+i))
			if err := s.InsertSession(id, "u", "user", "q"); err != nil {
				t.Errorf("concurrent InsertSession %d: %v", i, err)
			}
			if _, err := s.InsertToolEvent(id, "read", `{}`); err != nil {
				t.Errorf("concurrent InsertToolEvent %d: %v", i, err)
			}
			if err := s.UpdateSessionDone(id, "resp"); err != nil {
				t.Errorf("concurrent UpdateSessionDone %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestStoreListSessions(t *testing.T) {
	s := tempStore(t)

	s.InsertSession("s1", "u1", "alice", "kubernetes probe")
	s.InsertSession("s2", "u2", "bob", "docker volume")
	s.InsertSession("s3", "u1", "alice", "kubernetes service")
	s.UpdateSessionDone("s1", "")
	s.UpdateSessionDone("s2", "")
	s.UpdateSessionDone("s3", "")

	// Filter by user
	rows, total, err := s.ListSessions("alice", "", 0, 0, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("expected total=2 for alice, got %d", total)
	}
	_ = rows

	// Keyword search
	rows, total, err = s.ListSessions("", "kubernetes", 0, 0, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("expected total=2 for 'kubernetes', got %d", total)
	}
	_ = rows
}

func TestStoreGetSessionNotFound(t *testing.T) {
	s := tempStore(t)
	detail, err := s.GetSession("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if detail != nil {
		t.Errorf("expected nil for nonexistent session, got %+v", detail)
	}
}

func TestGetRecentHistory(t *testing.T) {
	s := tempStore(t)
	now := nowMs()
	window := int64(24 * 60 * 60 * 1000) // 24h in ms

	// Recent session (within window)
	s.InsertSession("h1", "u1", "alice", "query1")
	s.UpdateSessionDone("h1", "response1")

	// Old session (outside window)
	s.InsertSession("h2", "u1", "alice", "query2")
	s.UpdateSessionDone("h2", "response2")
	s.db.Exec(`UPDATE query_sessions SET started_at=? WHERE id=?`, now-window-1000, "h2")

	// Another user's session (should be excluded)
	s.InsertSession("h3", "u2", "bob", "query3")
	s.UpdateSessionDone("h3", "response3")

	since := now - window
	turns, err := s.GetRecentHistory("u1", since, 20)
	if err != nil {
		t.Fatalf("GetRecentHistory: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn within window, got %d", len(turns))
	}
	if turns[0].Query != "query1" {
		t.Errorf("expected query1, got %q", turns[0].Query)
	}
	if turns[0].Response != "response1" {
		t.Errorf("expected response1, got %q", turns[0].Response)
	}

	// Respect limit cap
	for i := 0; i < 5; i++ {
		id := "hlim" + string(rune('A'+i))
		s.InsertSession(id, "u1", "alice", "q")
		s.UpdateSessionDone(id, "r")
	}
	turns, err = s.GetRecentHistory("u1", since, 3)
	if err != nil {
		t.Fatalf("GetRecentHistory limit: %v", err)
	}
	if len(turns) != 3 {
		t.Errorf("expected 3 turns (limit), got %d", len(turns))
	}
}
