package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

type Job struct {
	ID      string `json:"id"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Message string `json:"message"`
	Repeat  bool   `json:"repeat"`
}

type Scheduler struct {
	mu       sync.Mutex
	jobs     map[string]*Job
	pty      *PTYManager
	savePath string
}

func newScheduler(pm *PTYManager) *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]*Job),
		pty:      pm,
		savePath: "schedules.json",
	}
}

func (s *Scheduler) addJob(job Job) string {
	b := make([]byte, 8)
	rand.Read(b)
	job.ID = hex.EncodeToString(b)
	s.mu.Lock()
	s.jobs[job.ID] = &job
	s.mu.Unlock()
	s.persist()
	return job.ID
}

func (s *Scheduler) deleteJob(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
	s.persist()
}

func (s *Scheduler) listJobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, *j)
	}
	return out
}

func (s *Scheduler) persist() {
	if s.savePath == "" {
		return
	}
	data, _ := json.MarshalIndent(s.listJobs(), "", "  ")
	os.WriteFile(s.savePath, data, 0600)
}

func (s *Scheduler) loadFromFile() {
	if s.savePath == "" {
		return
	}
	data, err := os.ReadFile(s.savePath)
	if err != nil {
		return
	}
	var jobs []Job
	if json.Unmarshal(data, &jobs) != nil {
		return
	}
	s.mu.Lock()
	for _, j := range jobs {
		jCopy := j
		s.jobs[j.ID] = &jCopy
	}
	s.mu.Unlock()
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for t := range ticker.C {
		s.mu.Lock()
		var toDelete []string
		for id, job := range s.jobs {
			if t.Hour() == job.Hour && t.Minute() == job.Minute {
				if s.pty != nil {
					s.pty.write([]byte(job.Message + "\n"))
				}
				if !job.Repeat {
					toDelete = append(toDelete, id)
				}
			}
		}
		for _, id := range toDelete {
			delete(s.jobs, id)
		}
		s.mu.Unlock()
		if len(toDelete) > 0 {
			s.persist()
		}
	}
}

func (s *Scheduler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/schedule":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.listJobs())
	case r.Method == http.MethodPost && r.URL.Path == "/schedule":
		var job Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.addJob(job)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodDelete && len(r.URL.Path) > len("/schedule/"):
		id := r.URL.Path[len("/schedule/"):]
		s.deleteJob(id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
