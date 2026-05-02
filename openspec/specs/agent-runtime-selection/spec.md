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

`loadAgentRuntime` SHALL accept `claude`, `opencode`, and `codex`. Any other value SHALL return an error.

#### Scenario: claude runtime carries claude-agent-acp

- **WHEN** `AGENT_RUNTIME=claude` (default)
- **THEN** the returned runtime has `ACPExecutable="claude-agent-acp"` and `ACPArgs=nil`

#### Scenario: opencode runtime carries `opencode acp --log-level WARN`

- **WHEN** `AGENT_RUNTIME=opencode`
- **THEN** the returned runtime has `ACPExecutable="opencode"` and `ACPArgs=[]string{"acp","--log-level","WARN"}`
- **AND** the `--log-level WARN` is required because `opencode acp` writes INFO logs to stdout by default, which would corrupt the JSON-RPC stream

#### Scenario: codex runtime carries `codex-acp`

- **WHEN** `AGENT_RUNTIME=codex`
- **THEN** the returned runtime has `ACPExecutable="codex-acp"` and `ACPArgs=nil`
- **AND** the runtime's `Name=="codex"`, `Command=="codex"`, `ArgsEnv=="CODEX_ARGS"`, `ProjectConfigDir==".codex"`, `ProjectConfigFile=="config.toml"`, `AssetDir=="/app/perch-codex"`, `SupportsHooks==false`
- **AND** `codex-acp` is the binary shipped by the npm package `@zed-industries/codex-acp` (Zed-maintained wrapper around OpenAI Codex)

> Verified by pre-flight: `npm view @zed-industries/codex-acp` exposes `bin: codex-acp` and `optionalDependencies` covering `linux-x64` + `linux-arm64` + darwin/win variants. Auth is via `OPENAI_API_KEY` env var inherited by the ACP subprocess.

#### Scenario: ACP path picks the runtime values

- **WHEN** chat-API, Discord, or Telegram acquires an ACP session
- **THEN** the spawned subprocess SHALL be `runtime.ACPExecutable` with `runtime.ACPArgs` prepended
- **AND** the legacy `ACP_EXECUTABLE` env var, if set, SHALL override only the executable; `ACP_EXECUTABLE_ARGS` (JSON array) MAY override the args

#### Scenario: invalid runtime rejected

- **WHEN** `AGENT_RUNTIME` is set to a value other than `claude`, `opencode`, or `codex`
- **THEN** `loadAgentRuntime` SHALL return a non-nil error of the form `unsupported AGENT_RUNTIME "<value>"`

### Requirement: AgentRuntime carries SupportsMCP and a model list

The `AgentRuntime` struct SHALL declare `SupportsMCP bool` and `Models []string` and `DefaultModel string` fields. `loadAgentRuntime` SHALL populate them per the configured runtime so that the chat-API can decide whether to pass `mcpServers`, and `GET /api/runtimes` can advertise a model list to the picker.

#### Scenario: claude runtime advertises MCP support and a Claude model list

- **WHEN** `AGENT_RUNTIME=claude`
- **THEN** the returned runtime has `SupportsMCP=true`, `Models` includes at least one `claude-*` id, and `DefaultModel` is set to the registry's recommended default

#### Scenario: codex / opencode / sst runtimes default to SupportsMCP=false until verified

- **WHEN** `AGENT_RUNTIME=codex` or `opencode` or `sst`
- **THEN** the returned runtime SHALL have `SupportsMCP=false` until the runtime is smoke-tested and the flag is flipped in the registry

### Requirement: Conversation-level runtime selection at POST /api/chat boundary

When `POST /api/chat` arrives with `(user_id, conversation_id)`, the server SHALL look up the conversation's `runtime` and `model` fields and use those values — not a server-wide default — to determine which `AgentRuntime` to use for the ACP session. If the conversation row's `runtime` is NULL (legacy data), the server SHALL backfill it with the current default and persist the row.

#### Scenario: Conversation runtime overrides server default

- **WHEN** the server default is `claude` but the conversation row has `runtime=opencode`
- **THEN** the ACP subprocess for this conversation is `opencode acp ...`, not `claude-agent-acp`

#### Scenario: NULL runtime is lazily backfilled

- **WHEN** a legacy conversation row has `runtime=NULL` and the user submits a message
- **THEN** the server sets the row's `runtime` and `model` to the current defaults via UPDATE, then proceeds with the ACP session

### Requirement: GET /api/runtimes advertises available runtimes and models

The endpoint `GET /api/runtimes` SHALL return the list of runtimes the server can spawn, with each entry containing `id`, `name`, `models[]`, `default_model`, `supports_mcp`. The list SHALL be derived from the runtime registry; the response SHALL be authentication-gated per the multi-user mode (single-user mode permits unauthenticated calls).

#### Scenario: Authenticated request returns runtime list

- **WHEN** an authenticated user calls `GET /api/runtimes`
- **THEN** the response is `{"runtimes":[{"id":"claude","name":"Claude","models":[...],"default_model":"...","supports_mcp":true}, ...]}`

#### Scenario: Unauthenticated request in multi-user mode is rejected

- **WHEN** the server runs in multi-user mode and a request without a valid session hits `GET /api/runtimes`
- **THEN** the server returns 401 (consistent with other `/api/*` endpoints)
