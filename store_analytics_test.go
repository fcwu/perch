package main

import (
	"testing"
	"time"
)

func TestGetUsageStatsNoData(t *testing.T) {
	s := tempStore(t)
	stats, err := s.GetUsageStats(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalQueries != 0 {
		t.Errorf("expected 0 queries, got %d", stats.TotalQueries)
	}
	if len(stats.Users) != 0 {
		t.Errorf("expected 0 users, got %d", len(stats.Users))
	}
	if len(stats.TopTools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(stats.TopTools))
	}
}

func TestGetUsageStatsWithData(t *testing.T) {
	s := tempStore(t)

	// Insert sessions
	s.InsertSession("s1", "u1", "alice", "q1")
	s.InsertSession("s2", "u1", "alice", "q2")
	s.InsertSession("s3", "u2", "bob", "q3")

	// Simulate some time passing so ended_at > started_at
	time.Sleep(2 * time.Millisecond)

	s.UpdateSessionDone("s1", "resp1")
	s.UpdateSessionDone("s2", "resp2")
	s.UpdateSessionDone("s3", "resp3")

	// Insert tool events
	s.InsertToolEvent("s1", "read", `{}`)
	s.InsertToolEvent("s1", "bash", `{}`)
	s.InsertToolEvent("s2", "read", `{}`)
	s.InsertToolEvent("s3", "read", `{}`)

	stats, err := s.GetUsageStats(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", stats.TotalQueries)
	}
	if len(stats.Users) != 2 {
		t.Errorf("expected 2 users, got %d", len(stats.Users))
	}
	// alice should be first (more queries)
	if stats.Users[0].Username != "alice" {
		t.Errorf("expected alice first, got %q", stats.Users[0].Username)
	}
	if stats.Users[0].QueryCount != 2 {
		t.Errorf("expected alice query_count=2, got %d", stats.Users[0].QueryCount)
	}
	if len(stats.TopTools) == 0 {
		t.Error("expected at least one top tool")
	}
	// read should be top tool
	if stats.TopTools[0].Tool != "read" {
		t.Errorf("expected 'read' as top tool, got %q", stats.TopTools[0].Tool)
	}
}
