package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// NotifyFunc is called when a sync error requires user notification.
// errType identifies the error category for debouncing; msg is the human-readable message.
type NotifyFunc func(errType string, msg string)

// WorkspaceSyncConfig holds configuration for the workspace auto-sync loop.
type WorkspaceSyncConfig struct {
	Enabled         bool
	Interval        time.Duration
	WorkspacePath   string
	GitToken        string
	GitUserName     string
	GitUserEmail    string
	NotifyChannelID string
	SyncSubmodules  bool
}

// LoadSyncConfig reads WorkspaceSyncConfig from environment variables.
func LoadSyncConfig() WorkspaceSyncConfig {
	cfg := WorkspaceSyncConfig{
		Interval:      60 * time.Second,
		WorkspacePath: "/workspace",
	}
	if v := os.Getenv("WORKSPACE_GIT_SYNC_ENABLED"); v == "true" || v == "1" {
		cfg.Enabled = true
	}
	if v := os.Getenv("WORKSPACE_GIT_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			cfg.Interval = d
		} else if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	}
	if v := os.Getenv("WORKSPACE_PATH"); v != "" {
		cfg.WorkspacePath = v
	}
	cfg.GitToken = os.Getenv("WORKSPACE_GIT_TOKEN")
	cfg.GitUserName = os.Getenv("WORKSPACE_GIT_USER_NAME")
	cfg.GitUserEmail = os.Getenv("WORKSPACE_GIT_USER_EMAIL")
	cfg.NotifyChannelID = os.Getenv("WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL")
	if v := os.Getenv("WORKSPACE_GIT_SYNC_SUBMODULES"); v == "true" || v == "1" {
		cfg.SyncSubmodules = true
	}
	return cfg
}

// isGitRepo reports whether path contains a .git directory.
func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

// isDirty reports whether the git repo at path has uncommitted changes.
func isDirty(path string) (bool, error) {
	out, err := runGit(context.Background(), path, "status", "--porcelain", "--ignore-submodules")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// isRebasing reports whether the repo at path is in a rebase-in-progress state.
func isRebasing(path string) bool {
	for _, marker := range []string{".git/rebase-merge", ".git/rebase-apply"} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

// runGit runs a git command in the given working directory and returns combined stdout+stderr.
// GIT_TERMINAL_PROMPT=0 prevents git from hanging waiting for interactive credential input.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	allArgs := append([]string{"-c", "safe.directory=*"}, args...)
	cmd := exec.CommandContext(ctx, "git", allArgs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// injectGitToken configures git credential store with the provided token.
// Only acts on HTTPS remotes; logs each step without revealing the token value.
func injectGitToken(token string, workspacePath string, logger *slog.Logger) error {
	if token == "" {
		logger.Info("workspace_sync: no WORKSPACE_GIT_TOKEN set, skipping credential injection")
		return nil
	}

	// Get the remote URL
	remoteURL, err := runGit(context.Background(), workspacePath, "remote", "get-url", "origin")
	if err != nil {
		logger.Warn("workspace_sync: credential injection: could not get remote URL", "err", err)
		return nil
	}
	remoteURL = strings.TrimSpace(remoteURL)

	// SSH remote: skip
	if strings.HasPrefix(remoteURL, "git@") || strings.HasPrefix(remoteURL, "ssh://") {
		logger.Warn("workspace_sync: git token ignored for SSH remote", "remote", remoteURL)
		return nil
	}

	// Parse host from HTTPS URL
	u, err := url.Parse(remoteURL)
	if err != nil || u.Host == "" {
		logger.Warn("workspace_sync: credential injection: cannot parse remote URL", "remote", remoteURL, "err", err)
		return nil
	}
	host := u.Host
	logger.Info("workspace_sync: injecting git token", "host", host)

	// Write to ~/.git-credentials
	credLine := fmt.Sprintf("https://x-token-auth:%s@%s\n", token, host)
	credPath := filepath.Join(os.Getenv("HOME"), ".git-credentials")
	existing, _ := os.ReadFile(credPath)
	// Replace existing entry for this host or append
	prefix := fmt.Sprintf("https://x-token-auth:")
	suffix := fmt.Sprintf("@%s", host)
	var kept []string
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.Contains(line, suffix) && strings.Contains(line, prefix) {
			continue
		}
		kept = append(kept, line)
	}
	content := strings.Join(kept, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	content += credLine
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		logger.Error("workspace_sync: credential injection: write ~/.git-credentials failed", "err", err)
		return err
	}
	logger.Info("workspace_sync: ~/.git-credentials updated", "host", host)

	// Set credential.helper = store
	if out, err := runGit(context.Background(), workspacePath, "config", "--global", "credential.helper", "store"); err != nil {
		logger.Error("workspace_sync: credential injection: set credential.helper failed", "output", out, "err", err)
		return err
	}
	logger.Info("workspace_sync: git credential helper set to store")
	logger.Info("workspace_sync: credential injection complete")
	return nil
}

// debouncer tracks last-notification time per error type to avoid spamming.
type debouncer struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
	window   time.Duration
}

func newDebouncer(window time.Duration) *debouncer {
	return &debouncer{lastSent: make(map[string]time.Time), window: window}
}

// allow returns true if a notification for errType should be sent now.
func (d *debouncer) allow(errType string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.lastSent[errType]; ok && time.Since(t) < d.window {
		return false
	}
	d.lastSent[errType] = time.Now()
	return true
}

// syncOnce performs one sync cycle: add → commit → pull --rebase → submodule update → push.
// All git command outputs are logged. Errors trigger notify if debounce allows.
func syncOnce(ctx context.Context, cfg WorkspaceSyncConfig, notify NotifyFunc, dbnc *debouncer, logger *slog.Logger) error {
	path := cfg.WorkspacePath
	logger.Debug("workspace_sync: starting sync")

	// Abort any in-progress rebase before attempting pull
	if isRebasing(path) {
		logger.Warn("workspace_sync: rebase in progress, aborting")
		out, err := runGit(ctx, path, "rebase", "--abort")
		logger.Info("workspace_sync: rebase abort output", "output", strings.TrimSpace(out))
		if err != nil {
			logger.Error("workspace_sync: rebase abort failed", "err", err)
			maybeNotify(notify, dbnc, "rebase_conflict",
				fmt.Sprintf("⚠️ git sync: rebase conflict detected in workspace and `git rebase --abort` failed: %v\nPlease resolve manually.", err))
		} else {
			logger.Info("workspace_sync: rebase abort succeeded")
			maybeNotify(notify, dbnc, "rebase_conflict",
				"⚠️ git sync: rebase conflict detected in workspace. Aborted the rebase — please pull manually and resolve conflicts.")
		}
		return fmt.Errorf("rebase was in progress, aborted")
	}

	// Auto-commit if dirty: add all changes and create a timestamped commit
	dirty, err := isDirty(path)
	if err != nil {
		logger.Error("workspace_sync: isDirty check failed", "err", err)
		return err
	}
	if dirty {
		logger.Debug("workspace_sync: dirty workspace, committing")
		addOut, addErr := runGit(ctx, path, "add", "-A")
		logger.Debug("workspace_sync: add output", "output", strings.TrimSpace(addOut))
		if addErr != nil {
			logger.Error("workspace_sync: add failed", "output", strings.TrimSpace(addOut), "err", addErr)
			return fmt.Errorf("git add -A: %w", addErr)
		}
		ts := time.Now().Format("2006-01-02 15:04:05")
		commitArgs := []string{}
		if cfg.GitUserName != "" {
			commitArgs = append(commitArgs, "-c", "user.name="+cfg.GitUserName)
		}
		if cfg.GitUserEmail != "" {
			commitArgs = append(commitArgs, "-c", "user.email="+cfg.GitUserEmail)
		}
		commitArgs = append(commitArgs, "commit", "-m", "auto-sync: "+ts)
		commitOut, commitErr := runGit(ctx, path, commitArgs...)
		logger.Info("workspace_sync: commit output", "output", strings.TrimSpace(commitOut))
		if commitErr != nil {
			logger.Error("workspace_sync: commit failed", "output", strings.TrimSpace(commitOut), "err", commitErr)
			return fmt.Errorf("git commit: %w", commitErr)
		}
	}

	// Pull --rebase; if working tree has residual changes (e.g. CRLF) that
	// would be overwritten, reset --hard and retry once.
	// Inject user identity so rebase commits don't fail with "Committer identity unknown".
	pullArgs := []string{}
	if cfg.GitUserName != "" {
		pullArgs = append(pullArgs, "-c", "user.name="+cfg.GitUserName)
	}
	if cfg.GitUserEmail != "" {
		pullArgs = append(pullArgs, "-c", "user.email="+cfg.GitUserEmail)
	}
	pullArgs = append(pullArgs, "pull", "--rebase")
	out, err := runGit(ctx, path, pullArgs...)
	logger.Info("workspace_sync: pull output", "output", strings.TrimSpace(out))
	if err != nil && strings.Contains(out, "local changes to the following files would be overwritten") {
		logger.Warn("workspace_sync: pull blocked by residual changes, resetting and retrying")
		resetOut, _ := runGit(ctx, path, "reset", "--hard")
		logger.Info("workspace_sync: reset output", "output", strings.TrimSpace(resetOut))
		out, err = runGit(ctx, path, pullArgs...)
		logger.Info("workspace_sync: pull retry output", "output", strings.TrimSpace(out))
	}
	if err != nil {
		logger.Error("workspace_sync: pull failed", "output", strings.TrimSpace(out), "err", err)
		maybeNotify(notify, dbnc, "pull_failed",
			fmt.Sprintf("⚠️ git sync: `git pull --rebase` failed in workspace:\n```\n%s\n```\nPlease resolve manually.", strings.TrimSpace(out)))
		return fmt.Errorf("git pull --rebase: %w", err)
	}

	// Update submodules if enabled
	if cfg.SyncSubmodules {
		subOut, subErr := runGit(ctx, path, "submodule", "update", "--init", "--recursive")
		logger.Info("workspace_sync: submodule update output", "output", strings.TrimSpace(subOut))
		if subErr != nil {
			logger.Error("workspace_sync: submodule update failed", "output", strings.TrimSpace(subOut), "err", subErr)
			maybeNotify(notify, dbnc, "submodule_failed",
				fmt.Sprintf("⚠️ git sync: `git submodule update --init --recursive` failed:\n```\n%s\n```", strings.TrimSpace(subOut)))
		}
	}

	// Push
	pushOut, pushErr := runGit(ctx, path, "push")
	logger.Info("workspace_sync: push output", "output", strings.TrimSpace(pushOut))
	if pushErr != nil {
		logger.Error("workspace_sync: push failed", "output", strings.TrimSpace(pushOut), "err", pushErr)
		maybeNotify(notify, dbnc, "push_failed",
			fmt.Sprintf("⚠️ git sync: `git push` failed in workspace:\n```\n%s\n```", strings.TrimSpace(pushOut)))
		return fmt.Errorf("git push: %w", pushErr)
	}

	logger.Debug("workspace_sync: sync complete")
	return nil
}

// maybeNotify calls notify if the debouncer allows it.
func maybeNotify(notify NotifyFunc, dbnc *debouncer, errType string, msg string) {
	if notify == nil {
		return
	}
	if dbnc.allow(errType) {
		notify(errType, msg)
	}
}

// StartWorkspaceSync starts the background git sync loop.
// It first checks that path is a git repo, then ticks at cfg.Interval.
// notify may be nil (errors are only logged).
func StartWorkspaceSync(ctx context.Context, cfg WorkspaceSyncConfig, logger *slog.Logger, notify NotifyFunc) {
	if !cfg.Enabled {
		return
	}
	if !isGitRepo(cfg.WorkspacePath) {
		logger.Info("workspace_sync: workspace is not a git repo, skipping sync", "path", cfg.WorkspacePath)
		return
	}
	logger.Info("workspace_sync: starting auto-sync loop", "path", cfg.WorkspacePath, "interval", cfg.Interval)

	dbnc := newDebouncer(5 * time.Minute)
	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Info("workspace_sync: stopping sync loop")
				return
			case <-ticker.C:
				if err := syncOnce(ctx, cfg, notify, dbnc, logger); err != nil {
					logger.Warn("workspace_sync: sync cycle failed", "err", err)
				}
			}
		}
	}()
}
