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

	if err := s.UpdateToolEventEnd(eventID, `{"result":"ok"}`); err != nil {
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
