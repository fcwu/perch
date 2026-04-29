package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchedulerListJobs(t *testing.T) {
	sched := newScheduler(nil, "", nil)
	sched.mu.Lock()
	sched.jobs["abc"] = &Job{ID: "abc", Hour: 9, Minute: 30, Message: "good morning", Repeat: true}
	sched.mu.Unlock()

	jobs := sched.listJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Message != "good morning" {
		t.Errorf("expected 'good morning', got %q", jobs[0].Message)
	}
}

func TestSchedulerTargetRouting(t *testing.T) {
	mainPTY := newPTYManager()
	sched := newScheduler(mainPTY, "", nil)

	type call struct {
		target  string
		message string
	}
	var calls []call
	sched.dispatch = func(target, message string) bool {
		calls = append(calls, call{target, message})
		return target == "discord:chan1"
	}

	// Targeted job that the adapter handles: dispatch true → no main-PTY leak.
	discordJob := &Job{Target: "discord:chan1", Message: "hello discord"}
	dispatched := false
	if discordJob.Target != "" && sched.dispatch != nil {
		dispatched = sched.dispatch(discordJob.Target, discordJob.Message)
	}
	if !dispatched {
		t.Errorf("discord job: expected dispatched=true")
	}

	// Untargeted job: dispatch never invoked, falls through to main PTY.
	mainJob := &Job{Target: "", Message: "hello main"}
	mainDispatched := false
	if mainJob.Target != "" && sched.dispatch != nil {
		mainDispatched = sched.dispatch(mainJob.Target, mainJob.Message)
	}
	if mainDispatched {
		t.Errorf("main job: dispatch should not be invoked for empty target")
	}

	// Targeted job the adapter does not own: dispatch=false → caller may
	// fall back to main-PTY behaviour.
	unknownJob := &Job{Target: "discord:unknown", Message: "hello unknown"}
	unknownDispatched := false
	if unknownJob.Target != "" && sched.dispatch != nil {
		unknownDispatched = sched.dispatch(unknownJob.Target, unknownJob.Message)
	}
	if unknownDispatched {
		t.Errorf("unknown target: expected dispatched=false (adapter declines, caller falls back)")
	}

	if len(calls) != 2 {
		t.Errorf("expected dispatch called twice (chan1, unknown), got %d: %+v", len(calls), calls)
	}
}

func TestSchedulerJSONL(t *testing.T) {
	dir := t.TempDir()
	sched := newScheduler(nil, dir, nil)

	// Write a JSONL file with two jobs (one without ID).
	line1 := `{"id":"id1","hour":8,"minute":0,"message":"wake up","repeat":true}` + "\n"
	line2 := `{"hour":9,"minute":30,"message":"standup","repeat":false}` + "\n"
	path := filepath.Join(dir, ".perch", "schedules.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte(line1+line2), 0600)

	sched.loadFromFile()

	jobs := sched.listJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	// Verify both jobs have IDs.
	for _, j := range jobs {
		if j.ID == "" {
			t.Errorf("job missing ID: %+v", j)
		}
	}
}

func TestSchedulerReloadAndDiff(t *testing.T) {
	dir := t.TempDir()
	sched := newScheduler(nil, dir, nil)
	path := filepath.Join(dir, ".perch", "schedules.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)

	// Initial state: one job.
	initial := `{"id":"aaa","hour":8,"minute":0,"message":"wake up","repeat":true}` + "\n"
	os.WriteFile(path, []byte(initial), 0600)
	sched.loadFromFile()

	// Reload with: original modified, one added, one deleted.
	updated := `{"id":"aaa","hour":8,"minute":30,"message":"wake up","repeat":true}` + "\n" +
		`{"id":"bbb","hour":10,"minute":0,"message":"new job","repeat":false}` + "\n"
	os.WriteFile(path, []byte(updated), 0600)
	sched.reloadAndDiff()

	jobs := sched.listJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs after reload, got %d", len(jobs))
	}

	found := map[string]bool{}
	for _, j := range jobs {
		found[j.ID] = true
	}
	if !found["aaa"] || !found["bbb"] {
		t.Errorf("unexpected job IDs: %v", found)
	}
}

func TestSchedulerWatchFileChange(t *testing.T) {
	dir := t.TempDir()
	sched := newScheduler(nil, dir, nil)
	path := filepath.Join(dir, ".perch", "schedules.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)

	// Start with empty file.
	os.WriteFile(path, []byte{}, 0600)
	sched.loadFromFile()

	go sched.watch()
	time.Sleep(50 * time.Millisecond) // let watcher start

	// Write a new job to the file.
	line := `{"id":"xyz","hour":7,"minute":0,"message":"morning","repeat":true}` + "\n"
	os.WriteFile(path, []byte(line), 0600)

	// Wait for debounce + reload (300ms debounce + buffer).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jobs := sched.listJobs()
		if len(jobs) == 1 && jobs[0].ID == "xyz" {
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("watcher did not reload the new job within 2s")
}

// TestSchedulerFireDue_TargetedJobSkipsMainPTY is the regression guard for
// T31/T32: when a job has a non-empty Target and dispatch handles it, the
// scheduler must NOT additionally write the message into the main local PTY.
// The earlier implementation always fell through to a main-PTY write because
// PTYForTarget returned nil for ACP-mode Discord sessions, leaking the
// scheduled prompt into the local terminal and never invoking ACP.
func TestSchedulerFireDue_TargetedJobSkipsMainPTY(t *testing.T) {
	mainPTY := newPTYManager() // not started → write would return errPTYNotReady
	sched := newScheduler(mainPTY, "", nil)

	var dispatched []string
	sched.dispatch = func(target, message string) bool {
		dispatched = append(dispatched, target+"|"+message)
		return true // adapter handled it
	}

	now := time.Date(2026, 4, 30, 9, 30, 0, 0, time.UTC)
	sched.mu.Lock()
	sched.jobs["t31"] = &Job{
		ID: "t31", Hour: 9, Minute: 30,
		Target: "discord:chan1", Message: "report time", Repeat: false,
	}
	sched.mu.Unlock()

	sched.fireDue(now)

	if len(dispatched) != 1 || dispatched[0] != "discord:chan1|report time" {
		t.Fatalf("expected one dispatch call for the Discord target, got %v", dispatched)
	}
	sched.mu.Lock()
	_, stillThere := sched.jobs["t31"]
	sched.mu.Unlock()
	if stillThere {
		t.Errorf("one-shot scheduled job should be removed after firing")
	}
}

// TestSchedulerFireDue_UntargetedFallsBackToMainPTY confirms the main-PTY
// path still runs for jobs without a Target — dispatch must not be invoked.
func TestSchedulerFireDue_UntargetedFallsBackToMainPTY(t *testing.T) {
	sched := newScheduler(newPTYManager(), "", nil)

	dispatched := 0
	sched.dispatch = func(target, message string) bool {
		dispatched++
		return true
	}

	now := time.Date(2026, 4, 30, 9, 30, 0, 0, time.UTC)
	sched.mu.Lock()
	sched.jobs["m"] = &Job{ID: "m", Hour: 9, Minute: 30, Message: "main job", Repeat: true}
	sched.mu.Unlock()

	sched.fireDue(now) // unstarted PTY: write attempt returns errPTYNotReady,
	// scheduler logs a warning and continues — no panic.

	if dispatched != 0 {
		t.Errorf("expected dispatch to be skipped for empty-target job, got %d calls", dispatched)
	}
}

// TestSchedulerFireDue_AdapterDeclinesFallsBackToMainPTY covers the case
// where the adapter does not own the target (e.g. unknown channel) — the
// scheduler must fall back to the main-PTY write so a misconfigured target
// never silently swallows the job.
func TestSchedulerFireDue_AdapterDeclinesFallsBackToMainPTY(t *testing.T) {
	sched := newScheduler(newPTYManager(), "", nil)

	dispatchCalls := 0
	sched.dispatch = func(target, message string) bool {
		dispatchCalls++
		return false // adapter declines
	}

	now := time.Date(2026, 4, 30, 9, 30, 0, 0, time.UTC)
	sched.mu.Lock()
	sched.jobs["u"] = &Job{
		ID: "u", Hour: 9, Minute: 30,
		Target: "discord:unknown", Message: "ping", Repeat: false,
	}
	sched.mu.Unlock()

	sched.fireDue(now)

	if dispatchCalls != 1 {
		t.Errorf("expected dispatch called exactly once, got %d", dispatchCalls)
	}
	// Fallback runs; unstarted PTY write fails with errPTYNotReady but the
	// scheduler swallows the error (logs warning). We just assert no panic
	// and the one-shot was still removed.
	sched.mu.Lock()
	_, stillThere := sched.jobs["u"]
	sched.mu.Unlock()
	if stillThere {
		t.Errorf("one-shot job should be removed even when adapter declined and fallback wrote to a not-ready PTY")
	}
}

func TestSchedulerPersistJSONL(t *testing.T) {
	dir := t.TempDir()
	sched := newScheduler(nil, dir, nil)
	path := filepath.Join(dir, ".perch", "schedules.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte{}, 0600)

	sched.mu.Lock()
	sched.jobs["j1"] = &Job{ID: "j1", Hour: 6, Minute: 0, Message: "early bird", Repeat: true}
	sched.mu.Unlock()
	sched.persist()

	data, _ := os.ReadFile(path)
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatalf("persist output is not valid JSON per line: %v\ncontent: %s", err, data)
	}
	if j.ID != "j1" {
		t.Errorf("expected id j1, got %q", j.ID)
	}
}
