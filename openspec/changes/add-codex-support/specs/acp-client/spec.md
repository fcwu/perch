## MODIFIED Requirements

### Requirement: ACP subprocess executable is selected by the active runtime

The ACP subprocess executable + args SHALL be selected by the active `AgentRuntime` (`runtime.ACPExecutable`, `runtime.ACPArgs`). The legacy `ACP_EXECUTABLE` env var SHALL act as a developer override of the executable only; `ACP_EXECUTABLE_ARGS` (JSON array) MAY override the args.

#### Scenario: Default executable comes from runtime

- **WHEN** the chat-API or IM adapter starts an ACP session
- **AND** neither `ACP_EXECUTABLE` nor `ACP_EXECUTABLE_ARGS` is set
- **THEN** the subprocess is `runtime.ACPExecutable runtime.ACPArgs...`
- **AND** for `AGENT_RUNTIME=claude` that is `claude-agent-acp` (no extra args)
- **AND** for `AGENT_RUNTIME=opencode` that is `opencode acp --log-level WARN`
- **AND** for `AGENT_RUNTIME=codex` that is `codex-acp` (no extra args)

#### Scenario: Codex runtime authenticates via OPENAI_API_KEY

- **WHEN** `AGENT_RUNTIME=codex` and the chat-API / IM spawns the ACP subprocess
- **THEN** the subprocess inherits container env including `OPENAI_API_KEY`
- **AND** if `OPENAI_API_KEY` is unset or invalid, `codex-acp` SHALL fail-fast (non-zero exit), and perch SHALL surface a chat-visible error rather than hang

#### Scenario: Env override of executable only

- **WHEN** `ACP_EXECUTABLE=/opt/my-fork/claude-agent-acp` is set
- **THEN** the spawned binary is `/opt/my-fork/claude-agent-acp` with `runtime.ACPArgs` unchanged

#### Scenario: Env override of both executable and args

- **WHEN** `ACP_EXECUTABLE=opencode` and `ACP_EXECUTABLE_ARGS=["acp","--log-level","DEBUG"]` are set
- **THEN** the spawned binary is `opencode acp --log-level DEBUG`

#### Scenario: Env override applies to codex runtime as well

- **WHEN** `AGENT_RUNTIME=codex` is set with `ACP_EXECUTABLE=/usr/local/bin/codex-acp` and `ACP_EXECUTABLE_ARGS=["--debug"]`
- **THEN** the spawned binary is `/usr/local/bin/codex-acp --debug` (override stacks identically across all runtimes)
