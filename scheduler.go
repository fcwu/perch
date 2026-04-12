package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Job struct {
	ID      string `json:"id"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Message string `json:"message"`
	Repeat  bool   `json:"repeat"`
	// Target identifies which PTY to write to when the job fires.
	// Empty string means the main PTY. "discord:<channelID>" routes to that Discord session.
	// Claude running inside a Discord PTY can read PERCH_SESSION_TARGET to obtain this value.
	Target string `json:"target,omitempty"`

	lastFiredAt time.Time // not persisted; prevents double-fire within the same minute
}

type Scheduler struct {
	mu        sync.Mutex
	jobs      map[string]*Job
	pty       *PTYManager
	ptyLookup func(target string) *PTYManager // optional; routes jobs with a non-empty Target
	onFire    func(target, message string)    // optional; called before writing to the PTY (e.g. Discord header)
	savePath  string                          // path to schedules.jsonl
	logger    *slog.Logger
	selfWrite bool // true while we are writing to prevent re-triggering watch
}

func newScheduler(pm *PTYManager, workdir string, logger *slog.Logger) *Scheduler {
	savePath := ""
	if workdir != "" {
		dir := workdir + "/.perch"
		if err := os.MkdirAll(dir, 0700); err == nil {
			savePath = dir + "/schedules.jsonl"
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		jobs:     make(map[string]*Job),
		pty:      pm,
		savePath: savePath,
		logger:   logger,
	}
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// readJobsFromFile parses a JSONL file into a slice of Jobs.
func readJobsFromFile(path string) ([]Job, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var jobs []Job
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var j Job
		if err := json.Unmarshal(line, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, scanner.Err()
}

// writeJobsToFile serialises jobs as JSONL (one JSON object per line).
func writeJobsToFile(path string, jobs []Job) error {
	var buf bytes.Buffer
	for _, j := range jobs {
		line, err := json.Marshal(j)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
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

// persist writes the current job list to disk as JSONL.
func (s *Scheduler) persist() {
	if s.savePath == "" {
		return
	}
	jobs := s.listJobs()
	s.mu.Lock()
	s.selfWrite = true
	s.mu.Unlock()
	if err := writeJobsToFile(s.savePath, jobs); err != nil {
		s.logger.Warn("schedule persist failed", "err", err)
		s.mu.Lock()
		s.selfWrite = false
		s.mu.Unlock()
	}
	// selfWrite is cleared in watch() after the fsnotify event is consumed.
}

// loadFromFile reads schedules.jsonl and populates s.jobs.
// Jobs without IDs get one auto-assigned and the file is rewritten.
func (s *Scheduler) loadFromFile() {
	if s.savePath == "" {
		return
	}
	jobs, err := readJobsFromFile(s.savePath)
	if err != nil {
		return
	}
	needsWrite := false
	s.mu.Lock()
	for i := range jobs {
		if jobs[i].ID == "" {
			jobs[i].ID = randomID()
			needsWrite = true
		}
		jCopy := jobs[i]
		s.jobs[jobs[i].ID] = &jCopy
	}
	s.mu.Unlock()
	if needsWrite {
		s.persist()
	}
}

// reloadAndDiff reads schedules.jsonl, diffs against current state, logs changes, and applies.
func (s *Scheduler) reloadAndDiff() {
	if s.savePath == "" {
		return
	}
	newJobs, err := readJobsFromFile(s.savePath)
	if err != nil {
		if os.IsNotExist(err) {
			newJobs = []Job{} // no file = no schedules
		} else {
			s.logger.Warn("schedule reload: cannot read file", "err", err)
			return
		}
	}

	// Auto-assign IDs to any jobs that lack one.
	needsWrite := false
	for i := range newJobs {
		if newJobs[i].ID == "" {
			newJobs[i].ID = randomID()
			needsWrite = true
		}
	}

	newMap := make(map[string]*Job, len(newJobs))
	for i := range newJobs {
		j := newJobs[i]
		newMap[j.ID] = &j
	}

	s.mu.Lock()
	old := s.jobs

	// Detect deletions and modifications.
	for id, oldJob := range old {
		newJob, exists := newMap[id]
		if !exists {
			s.logger.Info("schedule deleted",
				"id", id, "hour", oldJob.Hour, "minute", oldJob.Minute,
				"message", oldJob.Message, "target", oldJob.Target)
			continue
		}
		if oldJob.Hour != newJob.Hour || oldJob.Minute != newJob.Minute ||
			oldJob.Message != newJob.Message || oldJob.Repeat != newJob.Repeat ||
			oldJob.Target != newJob.Target {
			s.logger.Info("schedule modified",
				"id", id,
				"hour", newJob.Hour, "minute", newJob.Minute,
				"message", newJob.Message, "target", newJob.Target)
		}
	}

	// Detect additions.
	for id, newJob := range newMap {
		if _, exists := old[id]; !exists {
			s.logger.Info("schedule added",
				"id", id, "hour", newJob.Hour, "minute", newJob.Minute,
				"message", newJob.Message, "target", newJob.Target)
		}
		// Preserve lastFiredAt from old job to prevent double-fire on reload.
		if oldJob, exists := old[id]; exists {
			newJob.lastFiredAt = oldJob.lastFiredAt
		}
	}

	s.jobs = newMap
	s.mu.Unlock()

	if needsWrite {
		s.persist()
	}
}

// watch uses fsnotify to reload schedules whenever schedules.jsonl changes on disk.
func (s *Scheduler) watch() {
	if s.savePath == "" {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.logger.Warn("schedule watcher: cannot create", "err", err)
		return
	}
	defer watcher.Close()

	// Watch the directory — more reliable than watching the file directly,
	// as editors may rename/replace the file on save.
	dir := filepath.Dir(s.savePath)
	if err := watcher.Add(dir); err != nil {
		s.logger.Warn("schedule watcher: cannot watch dir", "dir", dir, "err", err)
		return
	}

	base := filepath.Base(s.savePath)
	var debounce *time.Timer

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != base {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// If we wrote the file ourselves, clear the flag and skip.
			s.mu.Lock()
			if s.selfWrite {
				s.selfWrite = false
				s.mu.Unlock()
				continue
			}
			s.mu.Unlock()

			// Debounce: wait 300 ms after the last event before reloading.
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(300*time.Millisecond, s.reloadAndDiff)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			s.logger.Warn("schedule watcher error", "err", err)
		}
	}
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for t := range ticker.C {
		s.mu.Lock()
		var toDelete []string
		for id, job := range s.jobs {
			if t.Hour() == job.Hour && t.Minute() == job.Minute {
				// skip if already fired within this minute
				if time.Since(job.lastFiredAt) < time.Minute {
					continue
				}
				job.lastFiredAt = t
				pm := s.pty
				if job.Target != "" && s.ptyLookup != nil {
					if found := s.ptyLookup(job.Target); found != nil {
						pm = found
					}
				}
				if pm != nil {
					if s.onFire != nil && job.Target != "" {
						s.onFire(job.Target, job.Message)
					}
					if err := pm.write([]byte(job.Message + "\r")); err != nil {
						s.logger.Warn("scheduler PTY write failed", "jobID", id, "target", job.Target, "err", err)
					}
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
