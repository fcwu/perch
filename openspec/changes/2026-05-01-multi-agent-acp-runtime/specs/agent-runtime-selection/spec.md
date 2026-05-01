## ADDED Requirements

### Requirement: AgentRuntime carries the ACP executable + args used by the ACP path

The `AgentRuntime` struct SHALL declare `ACPExecutable string` and `ACPArgs []string` fields. `loadAgentRuntime` SHALL populate them per the configured `AGENT_RUNTIME` so that the chat-API, Discord, and Telegram ACP subprocesses are picked from the runtime, not hard-coded.

#### Scenario: claude runtime carries claude-agent-acp

- **WHEN** `AGENT_RUNTIME=claude` (default)
- **THEN** the returned runtime has `ACPExecutable="claude-agent-acp"` and `ACPArgs=nil`

#### Scenario: opencode runtime carries `opencode acp`

- **WHEN** `AGENT_RUNTIME=opencode`
- **THEN** the returned runtime has `ACPExecutable="opencode"` and `ACPArgs=[]string{"acp"}`

#### Scenario: ACP path picks the runtime values

- **WHEN** chat-API, Discord, or Telegram acquires an ACP session
- **THEN** the spawned subprocess SHALL be `runtime.ACPExecutable` with `runtime.ACPArgs` prepended
- **AND** the legacy `ACP_EXECUTABLE` env var, if set, SHALL override only the executable; `ACP_EXECUTABLE_ARGS` (JSON array) MAY override the args
