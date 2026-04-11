package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchedulerAddAndList(t *testing.T) {
	sched := newScheduler(nil, "")
	sched.savePath = ""

	id := sched.addJob(Job{Hour: 9, Minute: 30, Message: "good morning", Repeat: true})
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	jobs := sched.listJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Message != "good morning" {
		t.Errorf("expected 'good morning', got %q", jobs[0].Message)
	}
}

func TestSchedulerDeleteJob(t *testing.T) {
	sched := newScheduler(nil, "")
	sched.savePath = ""

	id := sched.addJob(Job{Hour: 10, Minute: 0, Message: "standup", Repeat: true})
	sched.deleteJob(id)
	if len(sched.listJobs()) != 0 {
		t.Fatal("expected 0 jobs after delete")
	}
}

func TestSchedulerTargetRouting(t *testing.T) {
	mainPTY := newPTYManager()
	discordPTY := newPTYManager()

	sched := newScheduler(mainPTY, "")
	sched.savePath = ""
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

	// Job with a Discord target should resolve to the Discord PTY.
	discordJob := &Job{Target: "discord:chan1", Message: "hello discord"}
	if got := resolve(discordJob); got != discordPTY {
		t.Errorf("discord job: expected discordPTY, got %p (mainPTY=%p discordPTY=%p)", got, mainPTY, discordPTY)
	}

	// Job with no target should fall back to the main PTY.
	mainJob := &Job{Target: "", Message: "hello main"}
	if got := resolve(mainJob); got != mainPTY {
		t.Errorf("main job: expected mainPTY, got %p", got)
	}

	// Job with an unknown target should also fall back to the main PTY.
	unknownJob := &Job{Target: "discord:unknown", Message: "hello unknown"}
	if got := resolve(unknownJob); got != mainPTY {
		t.Errorf("unknown target: expected mainPTY, got %p", got)
	}
}

func TestSchedulerHTTPAPI(t *testing.T) {
	sched := newScheduler(nil, "")
	sched.savePath = ""
	ts := httptest.NewServer(sched)
	defer ts.Close()

	body, _ := json.Marshal(Job{Hour: 8, Minute: 0, Message: "wake up", Repeat: true})
	resp, err := http.Post(ts.URL+"/schedule", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /schedule: err=%v status=%d", err, resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/schedule")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /schedule: err=%v status=%d", err, resp.StatusCode)
	}
	var jobs []Job
	json.NewDecoder(resp.Body).Decode(&jobs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}
