package main

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestLogger(w io.Writer) *slog.Logger {
	if w == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// makeGitRepo creates a temporary git repo with an initial commit and returns its path.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// --- isGitRepo ---

func TestIsGitRepo_Valid(t *testing.T) {
	dir := makeGitRepo(t)
	if !isGitRepo(dir) {
		t.Errorf("expected isGitRepo(%q) = true", dir)
	}
}

func TestIsGitRepo_NonRepo(t *testing.T) {
	dir := t.TempDir()
	if isGitRepo(dir) {
		t.Errorf("expected isGitRepo(%q) = false for non-repo dir", dir)
	}
}

// --- isDirty ---

func TestIsDirty_Clean(t *testing.T) {
	dir := makeGitRepo(t)
	dirty, err := isDirty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("expected clean repo to not be dirty")
	}
}

func TestIsDirty_Dirty(t *testing.T) {
	dir := makeGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	dirty, err := isDirty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("expected repo with untracked file to be dirty")
	}
}

// --- isRebasing ---

func TestIsRebasing_NoRebase(t *testing.T) {
	dir := makeGitRepo(t)
	if isRebasing(dir) {
		t.Error("expected isRebasing = false for clean repo")
	}
}

func TestIsRebasing_RebaseMergeExists(t *testing.T) {
	dir := makeGitRepo(t)
	markerDir := filepath.Join(dir, ".git", "rebase-merge")
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if !isRebasing(dir) {
		t.Error("expected isRebasing = true when .git/rebase-merge exists")
	}
}

func TestIsRebasing_RebaseApplyExists(t *testing.T) {
	dir := makeGitRepo(t)
	markerDir := filepath.Join(dir, ".git", "rebase-apply")
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if !isRebasing(dir) {
		t.Error("expected isRebasing = true when .git/rebase-apply exists")
	}
}

// --- LoadSyncConfig ---

func TestLoadSyncConfig_Defaults(t *testing.T) {
	os.Unsetenv("WORKSPACE_GIT_SYNC_ENABLED")
	os.Unsetenv("WORKSPACE_GIT_SYNC_INTERVAL")
	os.Unsetenv("WORKSPACE_PATH")
	os.Unsetenv("WORKSPACE_GIT_TOKEN")
	os.Unsetenv("WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL")

	cfg := LoadSyncConfig()
	if cfg.Enabled {
		t.Error("expected Enabled = false by default")
	}
	if cfg.Interval != 60*time.Second {
		t.Errorf("expected Interval = 60s, got %v", cfg.Interval)
	}
	if cfg.WorkspacePath != "/workspace" {
		t.Errorf("expected WorkspacePath = /workspace, got %q", cfg.WorkspacePath)
	}
	if cfg.GitToken != "" {
		t.Error("expected GitToken empty by default")
	}
	if cfg.NotifyChannelID != "" {
		t.Error("expected NotifyChannelID empty by default")
	}
}

func TestLoadSyncConfig_EnvOverrides(t *testing.T) {
	t.Setenv("WORKSPACE_GIT_SYNC_ENABLED", "true")
	t.Setenv("WORKSPACE_GIT_SYNC_INTERVAL", "120")
	t.Setenv("WORKSPACE_PATH", "/tmp/ws")
	t.Setenv("WORKSPACE_GIT_TOKEN", "tok123")
	t.Setenv("WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL", "chan456")

	cfg := LoadSyncConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled = true")
	}
	if cfg.Interval != 120*time.Second {
		t.Errorf("expected Interval = 120s, got %v", cfg.Interval)
	}
	if cfg.WorkspacePath != "/tmp/ws" {
		t.Errorf("expected WorkspacePath = /tmp/ws, got %q", cfg.WorkspacePath)
	}
	if cfg.GitToken != "tok123" {
		t.Errorf("expected GitToken = tok123, got %q", cfg.GitToken)
	}
	if cfg.NotifyChannelID != "chan456" {
		t.Errorf("expected NotifyChannelID = chan456, got %q", cfg.NotifyChannelID)
	}
}

// --- injectGitToken ---

func TestInjectGitToken_Empty(t *testing.T) {
	dir := makeGitRepo(t)
	var logBuf strings.Builder
	logger := newTestLogger(&logBuf)
	if err := injectGitToken("", dir, logger); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logBuf.String(), "no WORKSPACE_GIT_TOKEN") {
		t.Error("expected log message about no token")
	}
}

func TestInjectGitToken_SSHRemote(t *testing.T) {
	dir := makeGitRepo(t)
	// Add an SSH remote
	c := exec.Command("git", "remote", "add", "origin", "git@github.com:user/repo.git")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("add remote: %v\n%s", err, out)
	}
	var logBuf strings.Builder
	logger := newTestLogger(&logBuf)
	if err := injectGitToken("mytoken", dir, logger); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logBuf.String(), "ignored for SSH remote") {
		t.Error("expected SSH remote warning in log")
	}
}

func TestInjectGitToken_HTTPSDoesNotLogToken(t *testing.T) {
	dir := makeGitRepo(t)
	c := exec.Command("git", "remote", "add", "origin", "https://github.com/user/repo.git")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("add remote: %v\n%s", err, out)
	}
	// Use a temp HOME so we don't pollute the real ~/.git-credentials
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	var logBuf strings.Builder
	logger := newTestLogger(&logBuf)
	const secret = "supersecrettoken"
	if err := injectGitToken(secret, dir, logger); err != nil {
		t.Fatal(err)
	}
	// Token must not appear in any log output
	if strings.Contains(logBuf.String(), secret) {
		t.Error("token value must not appear in log output")
	}
	// Credential file must contain the token
	credPath := filepath.Join(tmpHome, ".git-credentials")
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if !strings.Contains(string(data), secret) {
		t.Error("expected token in ~/.git-credentials")
	}
}

// --- debouncer ---

func TestDebouncer_AllowsFirst(t *testing.T) {
	d := newDebouncer(5 * time.Minute)
	if !d.allow("err_type") {
		t.Error("expected first notification to be allowed")
	}
}

func TestDebouncer_BlocksSecondWithinWindow(t *testing.T) {
	d := newDebouncer(5 * time.Minute)
	d.allow("err_type")
	if d.allow("err_type") {
		t.Error("expected second notification within window to be blocked")
	}
}

func TestDebouncer_AllowsAfterWindow(t *testing.T) {
	d := newDebouncer(10 * time.Millisecond)
	d.allow("err_type")
	time.Sleep(20 * time.Millisecond)
	if !d.allow("err_type") {
		t.Error("expected notification to be allowed after window expires")
	}
}

func TestDebouncer_IndependentTypes(t *testing.T) {
	d := newDebouncer(5 * time.Minute)
	d.allow("type_a")
	if !d.allow("type_b") {
		t.Error("expected different error type to be allowed independently")
	}
}

// --- syncOnce integration (local repo, no remote) ---

func TestSyncOnce_CleanRepoNoRemote(t *testing.T) {
	dir := makeGitRepo(t)
	notify := func(errType, msg string) {}
	dbnc := newDebouncer(5 * time.Minute)
	logger := newTestLogger(nil)
	cfg := WorkspaceSyncConfig{WorkspacePath: dir}

	// No remote configured — push will fail, but pull should fail first with a benign error
	err := syncOnce(t.Context(), cfg, notify, dbnc, logger)
	// We expect an error (no remote) but no panic
	if err == nil {
		t.Log("syncOnce succeeded unexpectedly (perhaps pull found nothing to do)")
	}
}
