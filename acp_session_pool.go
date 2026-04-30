package main

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	defaultPoolIdleTimeout = 15 * time.Minute
	defaultPoolPerUserMax  = 5
	defaultPoolGlobalMax   = 50
)

// acpPoolEntry holds one managed ACP subprocess.
type acpPoolEntry struct {
	key       string
	process   *ACPProcess
	refCount  int
	idleTimer *time.Timer
	lruElem   *list.Element
	lastUsed  time.Time
}

// ACPSessionPool manages a keyed set of long-lived ACP subprocesses.
// Keys follow the convention "<type>:<entity>:<id>", e.g.:
//
//	chat-api:user42:conv-abc
//	discord:channel:1234567890
//	telegram:chat:9876543210
//
// The pool enforces per-user limits (keyed on the first two segments) and a
// global limit, evicting idle (refCount==0) sessions via LRU when needed.
// All methods are safe for concurrent use.
type ACPSessionPool struct {
	executable      string
	workdir         string
	logger          *slog.Logger
	idleTimeout     time.Duration
	perUserMax      int
	globalMax       int
	processFactory  func() *ACPProcess // injectable for tests; nil → NewACPProcess

	mu       sync.Mutex
	sessions map[string]*acpPoolEntry
	lru      *list.List // front = most recently used
}

func newACPSessionPool(executable, workdir string, logger *slog.Logger) *ACPSessionPool {
	return &ACPSessionPool{
		executable:  executable,
		workdir:     workdir,
		logger:      logger,
		idleTimeout: defaultPoolIdleTimeout,
		perUserMax:  defaultPoolPerUserMax,
		globalMax:   defaultPoolGlobalMax,
		sessions:    make(map[string]*acpPoolEntry),
		lru:         list.New(),
	}
}

func (p *ACPSessionPool) newProcess() *ACPProcess {
	if p.processFactory != nil {
		return p.processFactory()
	}
	return NewACPProcess(p.executable, p.workdir, p.logger)
}

// Acquire returns the ACPProcess for key, starting one if it does not exist or
// has crashed. The reference count is incremented; caller must call Release.
func (p *ACPSessionPool) Acquire(ctx context.Context, key string) (*ACPProcess, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.sessions[key]; ok {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
			entry.idleTimer = nil
		}
		entry.refCount++
		entry.lastUsed = time.Now()
		p.lru.MoveToFront(entry.lruElem)

		if !entry.process.IsRunning() {
			// Replace the crashed process with a fresh one from the factory.
			newProc := p.newProcess()
			if err := newProc.EnsureRunning(ctx); err != nil {
				entry.refCount--
				return nil, fmt.Errorf("acp pool: restart %q: %w", key, err)
			}
			entry.process = newProc
		}
		return entry.process, nil
	}

	// Enforce per-user limit.
	prefix := acpUserPrefix(key)
	if p.perUserMax > 0 {
		count := 0
		for k := range p.sessions {
			if strings.HasPrefix(k, prefix) {
				count++
			}
		}
		if count >= p.perUserMax {
			p.evictByPrefix(prefix)
			count = 0
			for k := range p.sessions {
				if strings.HasPrefix(k, prefix) {
					count++
				}
			}
			if count >= p.perUserMax {
				return nil, fmt.Errorf("acp pool: per-user limit (%d) reached for %q", p.perUserMax, prefix)
			}
		}
	}

	// Enforce global limit.
	if p.globalMax > 0 && len(p.sessions) >= p.globalMax {
		p.evictGlobalLRU()
		if len(p.sessions) >= p.globalMax {
			return nil, fmt.Errorf("acp pool: global limit (%d) reached", p.globalMax)
		}
	}

	proc := p.newProcess()
	if err := proc.EnsureRunning(ctx); err != nil {
		return nil, fmt.Errorf("acp pool: start %q: %w", key, err)
	}

	entry := &acpPoolEntry{
		key:      key,
		process:  proc,
		refCount: 1,
		lastUsed: time.Now(),
	}
	entry.lruElem = p.lru.PushFront(key)
	p.sessions[key] = entry
	p.logger.Info("ACP pool: session created", "key", key, "total", len(p.sessions))
	return proc, nil
}

// Release decrements the reference count for key. When it reaches zero an idle
// timer is started; the subprocess will be terminated after idleTimeout.
func (p *ACPSessionPool) Release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.sessions[key]
	if !ok {
		return
	}
	entry.refCount--
	if entry.refCount > 0 {
		return
	}

	timeout := p.idleTimeout
	if timeout <= 0 {
		timeout = defaultPoolIdleTimeout
	}
	entry.idleTimer = time.AfterFunc(timeout, func() {
		p.expireKey(key)
	})
}

func (p *ACPSessionPool) expireKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.sessions[key]
	if !ok {
		return
	}
	if entry.refCount > 0 {
		return
	}
	entry.process.Stop()
	p.lru.Remove(entry.lruElem)
	delete(p.sessions, key)
	p.logger.Info("ACP pool: session expired (idle)", "key", key, "total", len(p.sessions))
}

// evictByPrefix removes the LRU idle entry whose key starts with prefix.
// Must be called with p.mu held.
func (p *ACPSessionPool) evictByPrefix(prefix string) {
	for elem := p.lru.Back(); elem != nil; elem = elem.Prev() {
		key := elem.Value.(string)
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		entry := p.sessions[key]
		if entry.refCount > 0 {
			continue
		}
		p.removeEntryLocked(entry, elem, "per-user LRU")
		return
	}
}

// evictGlobalLRU removes the globally least-recently-used idle entry.
// Must be called with p.mu held.
func (p *ACPSessionPool) evictGlobalLRU() {
	for elem := p.lru.Back(); elem != nil; elem = elem.Prev() {
		key := elem.Value.(string)
		entry := p.sessions[key]
		if entry.refCount > 0 {
			continue
		}
		p.removeEntryLocked(entry, elem, "global LRU")
		return
	}
}

func (p *ACPSessionPool) removeEntryLocked(entry *acpPoolEntry, elem *list.Element, reason string) {
	if entry.idleTimer != nil {
		entry.idleTimer.Stop()
		entry.idleTimer = nil
	}
	entry.process.Stop()
	p.lru.Remove(elem)
	delete(p.sessions, entry.key)
	p.logger.Info("ACP pool: session evicted", "key", entry.key, "reason", reason, "total", len(p.sessions))
}

// StopAll terminates all pooled subprocesses. Used during shutdown.
func (p *ACPSessionPool) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, entry := range p.sessions {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
		}
		entry.process.Stop()
	}
	p.sessions = make(map[string]*acpPoolEntry)
	p.lru.Init()
	p.logger.Info("ACP pool: all sessions stopped")
}

// acpUserPrefix returns the first two colon-separated segments of key followed
// by ":", used as the per-user bucket for limit counting.
// "chat-api:user42:conv-abc" → "chat-api:user42:"
func acpUserPrefix(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) >= 2 {
		return parts[0] + ":" + parts[1] + ":"
	}
	return key + ":"
}
