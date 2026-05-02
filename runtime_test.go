package main

import (
	"reflect"
	"testing"
)

func TestAgentRuntimeDefaultIsClaude(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", "")

	rt, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime returned error: %v", err)
	}
	if rt.Name != "claude" {
		t.Fatalf("expected default runtime claude, got %q", rt.Name)
	}
	if rt.Command != "claude" {
		t.Fatalf("expected claude command, got %q", rt.Command)
	}
}

func TestAgentRuntimeCanSelectOpenCode(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", "opencode")
	t.Setenv("OPENCODE_ARGS", "run --fast")

	rt, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime returned error: %v", err)
	}
	if rt.Name != "opencode" {
		t.Fatalf("expected runtime opencode, got %q", rt.Name)
	}
	if rt.Command != "opencode" {
		t.Fatalf("expected opencode command, got %q", rt.Command)
	}
	args := rt.MainArgs()
	if len(args) != 2 || args[0] != "run" || args[1] != "--fast" {
		t.Fatalf("expected OpenCode args from OPENCODE_ARGS, got %v", args)
	}
}

func TestAgentRuntimeRejectsInvalidValue(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", "unknown")

	_, err := loadAgentRuntime()
	if err == nil {
		t.Fatal("expected invalid runtime error, got nil")
	}
}

func TestRunAgentReturnsOpenCodeCommand(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", "opencode")
	rt, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime: %v", err)
	}
	cmd, args := rt.RunAgent("as-query", "what is X?", "/workspace")
	if cmd != "opencode" {
		t.Errorf("expected command 'opencode', got %q", cmd)
	}
	if len(args) != 4 || args[0] != "run" || args[1] != "--agent" || args[2] != "as-query" || args[3] != "what is X?" {
		t.Errorf("unexpected args: %v", args)
	}
}

// TestLoadAgentRuntime_ACPFields asserts that ACPExecutable + ACPArgs are
// populated correctly per AGENT_RUNTIME so the chat-API / IM ACP path picks
// the right binary instead of always spawning claude-agent-acp.
func TestLoadAgentRuntime_ACPFields(t *testing.T) {
	cases := []struct {
		name        string
		envValue    string
		wantExe     string
		wantArgs    []string
	}{
		{"default-claude", "", "claude-agent-acp", nil},
		{"claude-explicit", "claude", "claude-agent-acp", nil},
		{"opencode", "opencode", "opencode", []string{"acp", "--log-level", "WARN"}},
		{"codex", "codex", "codex-acp", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AGENT_RUNTIME", c.envValue)
			rt, err := loadAgentRuntime()
			if err != nil {
				t.Fatalf("loadAgentRuntime: %v", err)
			}
			if rt.ACPExecutable != c.wantExe {
				t.Errorf("ACPExecutable = %q, want %q", rt.ACPExecutable, c.wantExe)
			}
			if !reflect.DeepEqual(rt.ACPArgs, c.wantArgs) {
				t.Errorf("ACPArgs = %v, want %v", rt.ACPArgs, c.wantArgs)
			}
		})
	}
}

func TestAgentRuntimeCanSelectCodex(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", "codex")
	t.Setenv("CODEX_ARGS", "-c model=\"o3\"")

	rt, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime returned error: %v", err)
	}
	if rt.Name != "codex" {
		t.Fatalf("expected runtime codex, got %q", rt.Name)
	}
	if rt.Command != "codex" {
		t.Fatalf("expected codex command, got %q", rt.Command)
	}
	if rt.ProjectConfigDir != ".codex" || rt.ProjectConfigFile != "config.toml" {
		t.Fatalf("unexpected project config: %s/%s", rt.ProjectConfigDir, rt.ProjectConfigFile)
	}
	if rt.AssetDir != "/app/perch-codex" {
		t.Fatalf("unexpected AssetDir: %s", rt.AssetDir)
	}
	args := rt.MainArgs()
	if len(args) != 2 || args[0] != "-c" || args[1] != "model=\"o3\"" {
		t.Fatalf("expected Codex args from CODEX_ARGS, got %v", args)
	}
}

func TestAgentRuntimeKeepsRuntimeSpecificArgsIsolated(t *testing.T) {
	t.Setenv("CLAUDE_ARGS", "--model claude-opus-4-5")
	t.Setenv("OPENCODE_ARGS", "run --fast")

	t.Setenv("AGENT_RUNTIME", "claude")
	claude, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime for claude returned error: %v", err)
	}
	claudeArgs := claude.MainArgs()
	if len(claudeArgs) != 2 || claudeArgs[0] != "--model" || claudeArgs[1] != "claude-opus-4-5" {
		t.Fatalf("expected Claude args only, got %v", claudeArgs)
	}

	t.Setenv("AGENT_RUNTIME", "opencode")
	opencode, err := loadAgentRuntime()
	if err != nil {
		t.Fatalf("loadAgentRuntime for opencode returned error: %v", err)
	}
	opencodeArgs := opencode.MainArgs()
	if len(opencodeArgs) != 2 || opencodeArgs[0] != "run" || opencodeArgs[1] != "--fast" {
		t.Fatalf("expected OpenCode args only, got %v", opencodeArgs)
	}
}
