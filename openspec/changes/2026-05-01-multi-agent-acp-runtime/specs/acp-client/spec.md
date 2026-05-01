## MODIFIED Requirements

### Requirement: ACP base URL is configurable via environment variable

The ACP subprocess executable + args SHALL be selected by the active `AgentRuntime` (`runtime.ACPExecutable`, `runtime.ACPArgs`). The legacy `ACP_EXECUTABLE` env var SHALL act as a developer override of the executable only; a new `ACP_EXECUTABLE_ARGS` (JSON array) MAY override the args.

> Modification rationale: prior to this change the ACP path read `ACP_EXECUTABLE` directly with default `claude-agent-acp`, fully bypassing `AGENT_RUNTIME`. After this change the runtime is the source of truth, env vars are for dev override.

#### Scenario: Default executable comes from runtime

- **WHEN** the chat-API or IM adapter starts an ACP session
- **AND** neither `ACP_EXECUTABLE` nor `ACP_EXECUTABLE_ARGS` is set
- **THEN** the subprocess is `runtime.ACPExecutable runtime.ACPArgs...`
- **AND** for `AGENT_RUNTIME=claude` that is `claude-agent-acp` (no extra args)
- **AND** for `AGENT_RUNTIME=opencode` that is `opencode acp`

#### Scenario: Env override of executable only

- **WHEN** `ACP_EXECUTABLE=/opt/my-fork/claude-agent-acp` is set
- **THEN** the spawned binary is `/opt/my-fork/claude-agent-acp` with `runtime.ACPArgs` unchanged

#### Scenario: Env override of both executable and args

- **WHEN** `ACP_EXECUTABLE=opencode` and `ACP_EXECUTABLE_ARGS=["acp","--verbose"]` are set
- **THEN** the spawned binary is `opencode acp --verbose`
