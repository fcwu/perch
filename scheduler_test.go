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
	discordPTY := newPTYManager()

	sched := newScheduler(mainPTY, "", nil)
	sched.ptyLookup = func(target string) *PTYManager {
		if target == "discord:chan1" {
			return discordPTY
		}
		return nil
	}

	resolve := func(job *Job) *PTYManager {
		pm := sched.pty
		if job.Target != "" && sched.ptyLookup != nil {
			if found := sched.ptyLookup(job.Target); found != nil {
				pm = found
			}
		}
		return pm
	}

	discordJob := &Job{Target: "discord:chan1", Message: "hello discord"}
	if got := resolve(discordJob); got != discordPTY {
		t.Errorf("discord job: expected discordPTY, got %p (mainPTY=%p discordPTY=%p)", got, mainPTY, discordPTY)
	}

	mainJob := &Job{Target: "", Message: "hello main"}
	if got := resolve(mainJob); got != mainPTY {
		t.Errorf("main job: expected mainPTY, got %p", got)
	}

	unknownJob := &Job{Target: "discord:unknown", Message: "hello unknown"}
	if got := resolve(unknownJob); got != mainPTY {
		t.Errorf("unknown target: expected mainPTY, got %p", got)
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
