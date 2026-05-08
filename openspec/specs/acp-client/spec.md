## Requirements

### Requirement: ACP client creates a run and returns a run ID
The system SHALL provide an ACP client that sends a `POST /runs` request to the configured ACP server with the user message as input, and returns a run ID.

#### Scenario: Successful run creation
- **WHEN** `ACP_BASE_URL` is set and the client calls `CreateRun(ctx, message)`
- **THEN** the client sends `POST {ACP_BASE_URL}/runs` with the message payload and returns the run ID from the response

#### Scenario: ACP server unreachable
- **WHEN** the ACP server cannot be reached within the configured timeout
- **THEN** the client returns an error with a descriptive message

### Requirement: ACP client streams run output via SSE
The system SHALL support streaming run output by subscribing to `GET /runs/{id}/stream` (Server-Sent Events) and delivering text chunks to the caller.

#### Scenario: Streaming text output
- **WHEN** a run is in progress and the ACP server emits `MessageOutput` SSE events
- **THEN** the client collects the text content from each event and delivers them to the caller via a channel or callback

#### Scenario: Run completes successfully
- **WHEN** the ACP server emits a `RunCompleted` SSE event (or closes the stream)
- **THEN** the client signals completion and returns the accumulated output text

#### Scenario: Run fails with error
- **WHEN** the ACP server emits a `RunFailed` SSE event
- **THEN** the client returns an error with the failure reason

### Requirement: ACP client respects context cancellation
The system SHALL cancel the ACP run stream when the caller's context is cancelled.

#### Scenario: Context cancelled during streaming
- **WHEN** the caller cancels the context while a run is streaming
- **THEN** the client stops reading the SSE stream and returns a context cancellation error

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
