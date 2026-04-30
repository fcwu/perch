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
	SupportsHooks     bool
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
			SupportsHooks:     true,
		}, nil
	case "opencode":
		return AgentRuntime{
			Name:              "opencode",
			Command:           "opencode",
			ArgsEnv:           "OPENCODE_ARGS",
			ProjectConfigDir:  ".opencode",
			ProjectConfigFile: ".opencode.json",
			AssetDir:          "/app/perch-opencode",
			SupportsHooks:     false,
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
