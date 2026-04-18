package main

import "testing"

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
