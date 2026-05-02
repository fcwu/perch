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
	"strings"
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
	// Empty string means the main PTY. "discord:<channelID>" (or the looser
	// "discord:channel:<channelID>" form mirroring <#channelID> mentions)
	// routes to that Discord session.
	// Claude running inside a Discord PTY can read PERCH_SESSION_TARGET to obtain this value.
	Target string `json:"target,omitempty"`

	lastFiredAt time.Time // not persisted; prevents double-fire within the same minute
}

// chatJobState tracks fire-time bookkeeping for a chat_schedules row that has
// been merged into the scheduler's in-memory map.
type chatJobState struct {
	row         ChatSchedule
	lastFiredAt time.Time // not persisted; prevents double-fire within the same minute
}

type Scheduler struct {
	mu       sync.Mutex
	jobs     map[string]*Job
	pty      *PTYManager
	dispatch func(target, message string) bool // optional; if it returns true, scheduler skips the fallback PTY routing
	savePath string                            // path to schedules.jsonl
	logger   *slog.Logger
	selfWrite bool // true while we are writing to prevent re-triggering watch

	// store and chatFire are wired by main once the SQLite store + ACP chat
	// manager are constructed. chatFire is invoked by the ticker for matched
	// chat_schedules rows; it returns nil on success, an error on dispatch
	// failure (in which case the row is NOT deleted).
	store     *Store
	chatJobs  map[string]*chatJobState
	chatFire  func(sch ChatSchedule) error
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
		chatJobs: make(map[string]*chatJobState),
		pty:      pm,
		savePath: savePath,
		logger:   logger,
	}
}

// SetChatFire wires the chat-job dispatcher (typically the ACP chat session
// manager). The scheduler calls it from fireDue when a chat_schedules row
// matches the current time.
func (s *Scheduler) SetChatFire(store *Store, fire func(sch ChatSchedule) error) {
	s.mu.Lock()
	s.store = store
	s.chatFire = fire
	s.mu.Unlock()
}

// LoadChatSchedules reads chat_schedules from the store and replaces the
// in-memory chat job map. Called on boot and (by ReloadChatSchedules) after
// CRUD mutations.
func (s *Scheduler) LoadChatSchedules() {
	if s == nil {
		return
	}
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return
	}
	rows, err := store.LoadAllChatSchedules()
	if err != nil {
		s.logger.Warn("scheduler: load chat_schedules failed", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.chatJobs
	next := make(map[string]*chatJobState, len(rows))
	for _, r := range rows {
		state := &chatJobState{row: r}
		if prev, ok := old[r.ID]; ok {
			state.lastFiredAt = prev.lastFiredAt
		}
		next[r.ID] = state
	}
	s.chatJobs = next
}

// ReloadChatSchedules is the hot-reload path triggered by chat-schedule CRUD
// handlers. It re-queries the store and merges into the in-memory map.
func (s *Scheduler) ReloadChatSchedules() {
	s.LoadChatSchedules()
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
		s.fireDue(t)
	}
}

// fireDue runs all jobs whose Hour/Minute match t, dispatching each at most
// once per minute. Extracted for tests so the per-job path can be exercised
// without waiting on the 30s ticker.
func (s *Scheduler) fireDue(t time.Time) {
	s.mu.Lock()
	var toDelete []string
	for id, job := range s.jobs {
		if t.Hour() != job.Hour || t.Minute() != job.Minute {
			continue
		}
		if time.Since(job.lastFiredAt) < time.Minute {
			continue
		}
		job.lastFiredAt = t

		// chat: targets must never fall back to the main PTY — that
		// would leak the prompt into the local terminal and into a
		// runtime that isn't bound to the originating conversation.
		// Chat jobs live in chatJobs, but defensively skip the
		// JSONL-driven Job map's chat: targets too.
		if strings.HasPrefix(job.Target, "chat:") {
			s.logger.Warn("scheduler: legacy job has chat: target, skipping", "jobID", id, "target", job.Target)
			continue
		}

		// Targeted jobs (e.g. discord:<channelID>) are routed by the
		// adapter, which handles header posting and ACP/PTY dispatch
		// itself. Falling through to the main-PTY write would leak the
		// prompt into the local terminal (T31/T32 regression).
		dispatched := false
		if job.Target != "" && s.dispatch != nil {
			dispatched = s.dispatch(job.Target, job.Message)
		}

		if !dispatched && s.pty != nil {
			if err := s.pty.write([]byte(job.Message + "\n")); err != nil {
				s.logger.Warn("scheduler PTY write failed", "jobID", id, "target", job.Target, "err", err)
			}
		}
		if !job.Repeat {
			toDelete = append(toDelete, id)
		}
	}
	for _, id := range toDelete {
		delete(s.jobs, id)
	}

	// Chat-schedule jobs use the same ticker. Iterate after the JSONL jobs.
	chatFire := s.chatFire
	store := s.store
	var chatToDelete []string
	type chatFireJob struct {
		row     ChatSchedule
		isOneShot bool
		willDelete bool
	}
	var fires []chatFireJob
	nowMs := t.UnixMilli()
	for id, state := range s.chatJobs {
		row := state.row
		if !row.Enabled {
			continue
		}
		matched := false
		isOneShot := row.OneShotAt > 0
		if isOneShot {
			if row.OneShotAt <= nowMs {
				matched = true
			}
		} else if row.Hour != nil && row.Minute != nil {
			if t.Hour() == *row.Hour && t.Minute() == *row.Minute {
				matched = true
			}
		}
		if !matched {
			continue
		}
		if time.Since(state.lastFiredAt) < time.Minute {
			continue
		}
		state.lastFiredAt = t
		willDelete := isOneShot || !row.Repeat
		fires = append(fires, chatFireJob{row: row, isOneShot: isOneShot, willDelete: willDelete})
		if willDelete {
			chatToDelete = append(chatToDelete, id)
		}
	}
	s.mu.Unlock()

	// Dispatch chat fires outside the lock — fire functions may take seconds
	// to talk to the ACP subprocess.
	for _, fj := range fires {
		if chatFire == nil {
			s.logger.Warn("scheduler: chat fire requested but no dispatcher wired", "id", fj.row.ID)
			continue
		}
		if err := chatFire(fj.row); err != nil {
			s.logger.Warn("scheduler: chat fire failed", "id", fj.row.ID, "err", err)
			// Fire failed: undo the willDelete bookkeeping so the row
			// stays in the map and gets retried on the next tick.
			s.mu.Lock()
			if fj.willDelete {
				for i, did := range chatToDelete {
					if did == fj.row.ID {
						chatToDelete = append(chatToDelete[:i], chatToDelete[i+1:]...)
						break
					}
				}
			}
			s.mu.Unlock()
			continue
		}
		if store != nil {
			if err := store.TouchChatScheduleFired(fj.row.ID, nowMs); err != nil {
				s.logger.Warn("scheduler: touch chat_schedules.last_fired_at failed", "id", fj.row.ID, "err", err)
			}
		}
	}

	if len(chatToDelete) > 0 && store != nil {
		s.mu.Lock()
		for _, id := range chatToDelete {
			delete(s.chatJobs, id)
		}
		s.mu.Unlock()
		for _, id := range chatToDelete {
			if _, err := store.db.Exec(`DELETE FROM chat_schedules WHERE id=?`, id); err != nil {
				s.logger.Warn("scheduler: delete completed chat_schedules row failed", "id", id, "err", err)
			}
		}
	}

	if len(toDelete) > 0 {
		s.persist()
	}
}
