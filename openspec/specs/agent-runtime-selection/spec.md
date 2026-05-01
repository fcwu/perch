## MODIFIED Requirements

### Requirement: OpenCode subagent launch mode

The `AgentRuntime` for OpenCode SHALL support a `RunAgent(agentName, prompt, workdir string) (cmd, args)` helper that returns the command to launch OpenCode in non-interactive subagent mode.

#### Scenario: launch as-query agent with a prompt
- **WHEN** `UserSessionManager` starts a new session for a user query
- **THEN** it SHALL use `AgentRuntime.RunAgent("as-query", userQuery, workdir)` to obtain the command, which resolves to: `opencode run --agent as-query "<userQuery>"`

#### Scenario: subagent exits after completion
- **WHEN** the OpenCode subagent finishes processing the query
- **THEN** the PTY process SHALL exit with code 0; the server SHALL detect EOF and trigger session completion

#### Scenario: invalid agent name
- **WHEN** `RunAgent` is called with an agent name that does not exist in `.opencode/agents/`
- **THEN** OpenCode SHALL exit with a non-zero code; the server SHALL surface an error message to the user's WebSocket

### Requirement: AgentRuntime carries the ACP executable + args used by the ACP path

The `AgentRuntime` struct SHALL declare `ACPExecutable string` and `ACPArgs []string` fields. `loadAgentRuntime` SHALL populate them per the configured `AGENT_RUNTIME` so that the chat-API, Discord, and Telegram ACP subprocesses are picked from the runtime, not hard-coded.

#### Scenario: claude runtime carries claude-agent-acp

- **WHEN** `AGENT_RUNTIME=claude` (default)
- **THEN** the returned runtime has `ACPExecutable="claude-agent-acp"` and `ACPArgs=nil`

#### Scenario: opencode runtime carries `opencode acp --log-level WARN`

- **WHEN** `AGENT_RUNTIME=opencode`
- **THEN** the returned runtime has `ACPExecutable="opencode"` and `ACPArgs=[]string{"acp","--log-level","WARN"}`
- **AND** the `--log-level WARN` is required because `opencode acp` writes INFO logs to stdout by default, which would corrupt the JSON-RPC stream

> Verified by pre-flight on 2026-05-01: `printf '%s\n' '<initialize>' '<session/new>' | opencode acp --log-level WARN` returns clean JSON-RPC responses; without the flag, INFO log lines interleave.

#### Scenario: ACP path picks the runtime values

- **WHEN** chat-API, Discord, or Telegram acquires an ACP session
- **THEN** the spawned subprocess SHALL be `runtime.ACPExecutable` with `runtime.ACPArgs` prepended
- **AND** the legacy `ACP_EXECUTABLE` env var, if set, SHALL override only the executable; `ACP_EXECUTABLE_ARGS` (JSON array) MAY override the args
