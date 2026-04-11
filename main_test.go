package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// --- claudeArgs() unit tests ---

func TestClaudeArgsEmpty(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "")
	got := claudeArgs()
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestClaudeArgsWhitespaceOnly(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "   ")
	got := claudeArgs()
	if len(got) != 0 {
		t.Fatalf("expected empty slice for whitespace-only input, got %v", got)
	}
}

func TestClaudeArgsSingleFlag(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "--dangerously-skip-permissions")
	got := claudeArgs()
	want := []string{"--dangerously-skip-permissions"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestClaudeArgsFlagWithValue(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "--model claude-opus-4-5")
	got := claudeArgs()
	want := []string{"--model", "claude-opus-4-5"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] want %q, got %q", i, w, got[i])
		}
	}
}

func TestClaudeArgsMultipleFlags(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "--model claude-opus-4-5 --dangerously-skip-permissions")
	got := claudeArgs()
	want := []string{"--model", "claude-opus-4-5", "--dangerously-skip-permissions"}
	if len(got) != len(want) {
		t.Fatalf("want len=%d, got len=%d (%v)", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] want %q, got %q", i, w, got[i])
		}
	}
}

func TestClaudeArgsExtraSpaces(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "  --model   opus  ")
	got := claudeArgs()
	want := []string{"--model", "opus"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] want %q, got %q", i, w, got[i])
		}
	}
}

// --- PTY integration tests: verify args reach the child process ---

// TestPTYStartBroadcastsOutput verifies that start() broadcasts child process
// stdout via the subscriber channel.
func TestPTYStartBroadcastsOutput(t *testing.T) {
	pm := newPTYManager()
	ch, unsub := pm.subscribe()
	defer unsub()
	defer pm.stop()

	go pm.start("echo", []string{"pty-output-test"}, "", slog.Default())

	deadline := time.After(3 * time.Second)
	var buf []byte
	for {
		select {
		case data := <-ch:
			buf = append(buf, data...)
			if strings.Contains(string(buf), "pty-output-test") {
				return // success
			}
		case <-deadline:
			t.Fatalf("timed out waiting for echo output; received: %q", buf)
		}
	}
}

// TestPTYStartPassesArgsToProcess verifies that extra arguments are forwarded to
// the child process. We use "echo" as a stand-in for "claude": it prints its
// arguments to stdout, so we can assert they arrived.
func TestPTYStartPassesArgsToProcess(t *testing.T) {
	pm := newPTYManager()
	ch, unsub := pm.subscribe()
	defer unsub()
	defer pm.stop()

	marker := "perch-arg-marker-12345"
	go pm.start("echo", []string{marker}, "", slog.Default())

	deadline := time.After(3 * time.Second)
	var buf []byte
	for {
		select {
		case data := <-ch:
			buf = append(buf, data...)
			if strings.Contains(string(buf), marker) {
				return // args were forwarded to the process
			}
		case <-deadline:
			t.Fatalf("process did not receive args; output: %q", buf)
		}
	}
}

// TestPTYStartWorkdir verifies that the child process runs in the specified
// working directory by using "pwd" and checking the output.
func TestPTYStartWorkdir(t *testing.T) {
	pm := newPTYManager()
	ch, unsub := pm.subscribe()
	defer unsub()
	defer pm.stop()

	dir := t.TempDir()
	go pm.start("pwd", []string{}, dir, slog.Default())

	deadline := time.After(3 * time.Second)
	var buf []byte
	for {
		select {
		case data := <-ch:
			buf = append(buf, data...)
			if strings.Contains(string(buf), dir) {
				return // correct workdir
			}
		case <-deadline:
			t.Fatalf("expected workdir %q in output; got: %q", dir, buf)
		}
	}
}

// TestPTYStopPreventsRestart verifies that calling stop() before start() causes
// the start loop to exit immediately without spawning a process.
func TestPTYStopPreventsRestart(t *testing.T) {
	pm := newPTYManager()
	pm.stop() // stop before start

	done := make(chan struct{})
	go func() {
		pm.start("echo", []string{"should-not-run"}, "", slog.Default())
		close(done)
	}()

	select {
	case <-done:
		// start() returned immediately because done channel was already closed
	case <-time.After(2 * time.Second):
		t.Fatal("start() did not exit promptly after stop()")
	}
}
