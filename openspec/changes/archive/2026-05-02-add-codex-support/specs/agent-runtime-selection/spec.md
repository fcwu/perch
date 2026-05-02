## MODIFIED Requirements

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
