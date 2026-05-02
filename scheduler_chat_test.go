package main

import (
	"sync"
	"testing"
	"time"
)

// TestSchedulerChatFireDailyKeepsRow exercises the daily/repeat path: the row
// must remain in the table after fire.
func TestSchedulerChatFireDailyKeepsRow(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("c1", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hour := now.Hour()
	minute := now.Minute()
	sch := &ChatSchedule{
		ID:             "sch-1",
		UserID:         "user-a",
		ConversationID: "c1",
		Hour:           &hour,
		Minute:         &minute,
		Repeat:         true,
		Prompt:         "ping",
		Enabled:        true,
	}
	if err := store.InsertChatSchedule(sch); err != nil {
		t.Fatal(err)
	}

	sched := newScheduler(nil, "", nil)
	var fired []ChatSchedule
	var mu sync.Mutex
	sched.SetChatFire(store, func(row ChatSchedule) error {
		mu.Lock()
		fired = append(fired, row)
		mu.Unlock()
		return nil
	})
	sched.LoadChatSchedules()
	sched.fireDue(now)

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 1 {
		t.Fatalf("expected 1 chat fire, got %d", len(fired))
	}
	rows, _ := store.LoadAllChatSchedules()
	if len(rows) != 1 {
		t.Errorf("expected daily-repeat row to be retained, got %d", len(rows))
	}
}

// TestSchedulerChatFireOneShotDeletesRow exercises the one-shot path: the row
// is deleted from the store after firing.
func TestSchedulerChatFireOneShotDeletesRow(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("c1", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sch := &ChatSchedule{
		ID:             "sch-os",
		UserID:         "user-a",
		ConversationID: "c1",
		OneShotAt:      now.UnixMilli() - 1, // already due
		Prompt:         "ping",
		Enabled:        true,
	}
	if err := store.InsertChatSchedule(sch); err != nil {
		t.Fatal(err)
	}

	sched := newScheduler(nil, "", nil)
	sched.SetChatFire(store, func(row ChatSchedule) error { return nil })
	sched.LoadChatSchedules()
	sched.fireDue(now)

	rows, _ := store.LoadAllChatSchedules()
	if len(rows) != 0 {
		t.Errorf("expected one-shot row to be deleted after fire, got %d rows", len(rows))
	}
}

// TestSchedulerChatFireDisabledSkipped confirms enabled=0 rows are skipped.
func TestSchedulerChatFireDisabledSkipped(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("c1", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hour := now.Hour()
	minute := now.Minute()
	sch := &ChatSchedule{
		ID:             "sch-d",
		UserID:         "user-a",
		ConversationID: "c1",
		Hour:           &hour,
		Minute:         &minute,
		Repeat:         true,
		Prompt:         "ping",
		Enabled:        false,
	}
	if err := store.InsertChatSchedule(sch); err != nil {
		t.Fatal(err)
	}

	sched := newScheduler(nil, "", nil)
	var fired int
	sched.SetChatFire(store, func(row ChatSchedule) error {
		fired++
		return nil
	})
	sched.LoadChatSchedules()
	sched.fireDue(now)
	if fired != 0 {
		t.Errorf("expected 0 fires for disabled row, got %d", fired)
	}
}

// TestSchedulerChatFireFailureKeepsRow confirms a fire error keeps the row
// for a future retry — we don't lose the schedule on transient errors.
func TestSchedulerChatFireFailureKeepsRow(t *testing.T) {
	store := openTestStore(t)
	if err := store.InsertConversation("c1", "user-a", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sch := &ChatSchedule{
		ID:             "sch-fail",
		UserID:         "user-a",
		ConversationID: "c1",
		OneShotAt:      now.UnixMilli() - 1,
		Prompt:         "ping",
		Enabled:        true,
	}
	if err := store.InsertChatSchedule(sch); err != nil {
		t.Fatal(err)
	}
	sched := newScheduler(nil, "", nil)
	sched.SetChatFire(store, func(row ChatSchedule) error {
		return &fireErr{}
	})
	sched.LoadChatSchedules()
	sched.fireDue(now)

	rows, _ := store.LoadAllChatSchedules()
	if len(rows) != 1 {
		t.Errorf("expected failed-fire row to be retained, got %d", len(rows))
	}
}

type fireErr struct{}

func (e *fireErr) Error() string { return "fire failed" }
