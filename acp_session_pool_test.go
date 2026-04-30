package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// newPreRunningACPProcess returns an ACPProcess that is already "running"
// (pipes wired, readLoop started) without performing the ACP handshake.
// Stopping the process closes the pipes so readLoop exits cleanly.
func newPreRunningACPProcess(t *testing.T) *ACPProcess {
	t.Helper()
	stdoutR, _ := io.Pipe() // server side: write to trigger readLoop exit
	stdinR, stdinW := io.Pipe()

	proc := &ACPProcess{
		executable: "fake-acp",
		workdir:    t.TempDir(),
		logger:     slog.Default(),
		pending:    make(map[int64]chan acpMsg),
		stdinW:     stdinW,
		running:    true,
		sessionID:  "fake-session",
	}
	go proc.readLoop(stdoutR)
	// Drain stdin so writes don't block.
	go io.Copy(io.Discard, stdinR) //nolint:errcheck
	t.Cleanup(func() { _ = stdoutR.Close() })
	return proc
}

// newPoolWithFakeProcs returns a pool whose processFactory returns pre-wired
// "already running" ACPProcess instances, so no real subprocess is needed.
func newPoolWithFakeProcs(t *testing.T) *ACPSessionPool {
	t.Helper()
	pool := newACPSessionPool("", t.TempDir(), slog.Default())
	var callCount atomic.Int32
	pool.processFactory = func() *ACPProcess {
		callCount.Add(1)
		return newPreRunningACPProcess(t)
	}
	return pool
}

func TestACPSessionPool_AcquireRelease(t *testing.T) {
	pool := newPoolWithFakeProcs(t)
	pool.idleTimeout = 10 * time.Second

	ctx := context.Background()
	proc, err := pool.Acquire(ctx, "test:user:conv1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if proc == nil {
		t.Fatal("Acquire returned nil process")
	}

	pool.mu.Lock()
	entry := pool.sessions["test:user:conv1"]
	pool.mu.Unlock()
	if entry == nil {
		t.Fatal("session not in pool after Acquire")
	}
	if entry.refCount != 1 {
		t.Fatalf("refCount want 1, got %d", entry.refCount)
	}

	pool.Release("test:user:conv1")
	pool.mu.Lock()
	entry = pool.sessions["test:user:conv1"]
	pool.mu.Unlock()
	if entry == nil {
		t.Fatal("session removed before idle timeout")
	}
	if entry.refCount != 0 {
		t.Fatalf("refCount after Release want 0, got %d", entry.refCount)
	}
	if entry.idleTimer == nil {
		t.Fatal("idle timer not set after Release with refCount=0")
	}
}

func TestACPSessionPool_SameKeyReturnsExistingProcess(t *testing.T) {
	pool := newPoolWithFakeProcs(t)

	ctx := context.Background()
	proc1, err := pool.Acquire(ctx, "discord:channel:999")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	proc2, err := pool.Acquire(ctx, "discord:channel:999")
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if proc1 != proc2 {
		t.Fatal("second Acquire returned different process for same key")
	}

	pool.mu.Lock()
	rc := pool.sessions["discord:channel:999"].refCount
	pool.mu.Unlock()
	if rc != 2 {
		t.Fatalf("refCount after two Acquires want 2, got %d", rc)
	}

	pool.Release("discord:channel:999")
	pool.Release("discord:channel:999")
}

func TestACPSessionPool_IdleTimeout(t *testing.T) {
	pool := newPoolWithFakeProcs(t)
	pool.idleTimeout = 50 * time.Millisecond // short for test

	ctx := context.Background()
	if _, err := pool.Acquire(ctx, "test:u:c"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	pool.Release("test:u:c")

	// Session should still exist immediately after release.
	pool.mu.Lock()
	_, exists := pool.sessions["test:u:c"]
	pool.mu.Unlock()
	if !exists {
		t.Fatal("session removed immediately on release (want idle timer)")
	}

	time.Sleep(200 * time.Millisecond)
	pool.mu.Lock()
	_, exists = pool.sessions["test:u:c"]
	pool.mu.Unlock()
	if exists {
		t.Fatal("session still present after idle timeout")
	}
}

func TestACPSessionPool_CrashRestart(t *testing.T) {
	pool := newPoolWithFakeProcs(t)

	ctx := context.Background()
	proc1, err := pool.Acquire(ctx, "test:u:c")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Simulate crash.
	proc1.Stop()

	// Re-acquire the same key: pool should replace with a new process.
	proc2, err := pool.Acquire(ctx, "test:u:c")
	if err != nil {
		t.Fatalf("Acquire after crash: %v", err)
	}
	if proc2 == nil {
		t.Fatal("Acquire after crash returned nil")
	}
	if proc2 == proc1 {
		t.Fatal("expected new *ACPProcess after crash, got same pointer")
	}
	if !proc2.IsRunning() {
		t.Fatal("replacement process should be running")
	}
	pool.Release("test:u:c")
	pool.Release("test:u:c")
}

func TestACPSessionPool_PerUserLimit(t *testing.T) {
	pool := newPoolWithFakeProcs(t)
	pool.perUserMax = 2

	ctx := context.Background()
	if _, err := pool.Acquire(ctx, "chat-api:userX:conv1"); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if _, err := pool.Acquire(ctx, "chat-api:userX:conv2"); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	// Third session for same user should fail (no idle to evict).
	_, err := pool.Acquire(ctx, "chat-api:userX:conv3")
	if err == nil {
		t.Fatal("expected per-user limit error, got nil")
	}

	// Different user should succeed.
	if _, err := pool.Acquire(ctx, "chat-api:userY:conv1"); err != nil {
		t.Fatalf("Acquire different user: %v", err)
	}
}

func TestACPSessionPool_PerUserLimitEvictsIdle(t *testing.T) {
	pool := newPoolWithFakeProcs(t)
	pool.perUserMax = 2
	pool.idleTimeout = 10 * time.Second

	ctx := context.Background()
	if _, err := pool.Acquire(ctx, "chat-api:userX:conv1"); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if _, err := pool.Acquire(ctx, "chat-api:userX:conv2"); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	// Release conv1 so it becomes idle (evictable).
	pool.Release("chat-api:userX:conv1")

	// Now conv3 should succeed by evicting conv1.
	if _, err := pool.Acquire(ctx, "chat-api:userX:conv3"); err != nil {
		t.Fatalf("Acquire 3 after eviction: %v", err)
	}

	pool.mu.Lock()
	_, conv1Exists := pool.sessions["chat-api:userX:conv1"]
	pool.mu.Unlock()
	if conv1Exists {
		t.Fatal("conv1 should have been evicted")
	}
}

func TestACPSessionPool_GlobalLimit(t *testing.T) {
	pool := newPoolWithFakeProcs(t)
	pool.globalMax = 2
	pool.perUserMax = 100 // disable per-user limit for this test

	ctx := context.Background()
	if _, err := pool.Acquire(ctx, "a:u:1"); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if _, err := pool.Acquire(ctx, "b:u:2"); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	// Third should fail (no idle to evict — both active).
	if _, err := pool.Acquire(ctx, "c:u:3"); err == nil {
		t.Fatal("expected global limit error, got nil")
	}
}

func TestACPSessionPool_StopAll(t *testing.T) {
	pool := newPoolWithFakeProcs(t)

	ctx := context.Background()
	if _, err := pool.Acquire(ctx, "a:u:1"); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if _, err := pool.Acquire(ctx, "b:u:2"); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}

	pool.StopAll()

	pool.mu.Lock()
	n := len(pool.sessions)
	pool.mu.Unlock()
	if n != 0 {
		t.Fatalf("StopAll: want 0 sessions, got %d", n)
	}
}

func TestACPUserPrefix(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"chat-api:user42:conv-abc", "chat-api:user42:"},
		{"discord:channel:1234567890", "discord:channel:"},
		{"telegram:chat:9876", "telegram:chat:"},
		{"nocoions", "nocoions:"},
	}
	for _, c := range cases {
		if got := acpUserPrefix(c.key); got != c.want {
			t.Errorf("acpUserPrefix(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}
