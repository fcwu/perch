package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AgentRuntime struct {
	Name              string
	Command           string
	ArgsEnv           string
	DefaultEnv        []string
	ProjectConfigDir  string
	ProjectConfigFile string
	AssetDir          string

	// ACPExecutable is the binary spawned for chat-API / Discord / Telegram
	// ACP sessions when this runtime is active. ACPArgs are passed before any
	// runtime-level ACP_EXECUTABLE_ARGS override; together they form the
	// argv used by exec.Command.
	ACPExecutable string
	ACPArgs       []string
}

func loadAgentRuntime() (AgentRuntime, error) {
	name := strings.TrimSpace(os.Getenv("AGENT_RUNTIME"))
	if name == "" {
		name = "claude"
	}

	switch name {
	case "claude":
		return AgentRuntime{
			Name:              "claude",
			Command:           "claude",
			ArgsEnv:           "CLAUDE_ARGS",
			DefaultEnv:        []string{"CLAUDE_CODE_NO_FLICKER=1", "CLAUDE_CODE_DISABLE_MOUSE=1"},
			ProjectConfigDir:  ".claude",
			ProjectConfigFile: "settings.json",
			AssetDir:          "/app/perch-claude",
			ACPExecutable:     "claude-agent-acp",
			ACPArgs:           nil,
		}, nil
	case "opencode":
		return AgentRuntime{
			Name:              "opencode",
			Command:           "opencode",
			ArgsEnv:           "OPENCODE_ARGS",
			ProjectConfigDir:  ".opencode",
			ProjectConfigFile: ".opencode.json",
			AssetDir:          "/app/perch-opencode",
			ACPExecutable:     "opencode",
			// --log-level WARN: opencode acp writes INFO logs to stdout by
			// default, which corrupts the JSON-RPC stream perch reads.
			ACPArgs: []string{"acp", "--log-level", "WARN"},
		}, nil
	case "codex":
		return AgentRuntime{
			Name:              "codex",
			Command:           "codex",
			ArgsEnv:           "CODEX_ARGS",
			ProjectConfigDir:  ".codex",
			ProjectConfigFile: "config.toml",
			AssetDir:          "/app/perch-codex",
			// codex-acp is the Zed-maintained ACP wrapper around OpenAI Codex
			// (npm @zed-industries/codex-acp). Auth via OPENAI_API_KEY env var
			// inherited by the subprocess. No subcommand or log-level flag
			// needed — pre-flight verified stdout is clean JSON-RPC.
			ACPExecutable: "codex-acp",
			ACPArgs:       nil,
		}, nil
	default:
		return AgentRuntime{}, fmt.Errorf("unsupported AGENT_RUNTIME %q", name)
	}
}

func (r AgentRuntime) MainArgs() []string {
	return strings.Fields(os.Getenv(r.ArgsEnv))
}

func (r AgentRuntime) SessionArgs(target string) []string {
	if r.Name == "claude" {
		return []string{"--permission-mode", "bypassPermissions", "--name", target}
	}
	return r.MainArgs()
}

func (r AgentRuntime) SessionEnv(target string) []string {
	if target == "" {
		return nil
	}
	return []string{"PERCH_SESSION_TARGET=" + target}
}

func (r AgentRuntime) ProjectConfigPath(workdir string) string {
	if workdir == "" {
		return ""
	}
	if r.Name == "opencode" {
		return filepath.Join(workdir, r.ProjectConfigFile)
	}
	return filepath.Join(workdir, r.ProjectConfigDir, r.ProjectConfigFile)
}

// RunAgent returns the command and args to launch an agent session with the given prompt.
func (r AgentRuntime) RunAgent(agentName, prompt, _ string) (string, []string) {
	return r.Command, []string{"run", "--agent", agentName, prompt}
}
