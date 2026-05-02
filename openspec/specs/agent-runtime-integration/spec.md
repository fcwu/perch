## ADDED Requirements

### Requirement: Runtime-specific project configuration assets are synced into the workspace
The system SHALL copy or prepare runtime-specific project configuration assets in the active runtime's project-level configuration directory without modifying unrelated runtime directories.

#### Scenario: Claude assets remain in .claude
- **WHEN** the active runtime is `claude`
- **THEN** Perch syncs bundled Claude assets into the workspace's `.claude` project configuration path

#### Scenario: OpenCode assets use OpenCode config path
- **WHEN** the active runtime is `opencode`
- **THEN** Perch syncs bundled OpenCode assets into the corresponding OpenCode project configuration path instead of `.claude`

### Requirement: Runtime-specific hook or callback integration supports IM completion flow
The active runtime MUST provide a callback path that allows Perch to deliver IM notification lifecycle events needed for Discord or other IM integrations.

#### Scenario: OpenCode runtime triggers Perch callback flow
- **WHEN** the active runtime is `opencode` and an IM-triggered request executes
- **THEN** Perch receives runtime callbacks or an equivalent completion signal that allows it to notify the IM integration of work progress or completion

#### Scenario: Claude callback behavior preserved
- **WHEN** the active runtime is `claude`
- **THEN** the existing hook callback flow to `/hook` continues to work without requiring behavior changes from current Claude users

### Requirement: Discord response flow works with the selected runtime
The system MUST preserve Discord request/response behavior when the active runtime is changed, including writing inbound messages to PTY sessions and sending a completion reply back to Discord.

#### Scenario: OpenCode responds back to Discord
- **WHEN** a Discord message is handled while the active runtime is `opencode`
- **THEN** the message is written into an OpenCode-backed PTY session and Perch sends the runtime's completion response back to Discord

#### Scenario: Scheduler reply flow preserved for runtime
- **WHEN** a scheduler job targets a Discord session under the selected runtime
- **THEN** Perch posts the scheduler header and later delivers the runtime completion output back to the same Discord channel

### Requirement: Runtime prerequisites are installed in the runtime image

The runtime image SHALL ship binaries for every supported `AGENT_RUNTIME` so the runtime can be switched without rebuilding.

- The OpenCode binary SHALL be sourced from `sst/opencode` GitHub releases, with the asset matching the host architecture (amd64 → `opencode-linux-x64.tar.gz`, arm64 → `opencode-linux-arm64.tar.gz`).
- The Codex ACP adapter SHALL be installed via npm (`@zed-industries/codex-acp`); npm SHALL select the correct platform binary automatically through the package's `optionalDependencies` (`@zed-industries/codex-acp-linux-x64` on amd64, `@zed-industries/codex-acp-linux-arm64` on arm64).

#### Scenario: amd64 host installs opencode-linux-x64

- **WHEN** the image is built on an amd64 host
- **THEN** Dockerfile detects `dpkg --print-architecture == amd64` and downloads `opencode-linux-x64.tar.gz` from `sst/opencode/releases/latest`

#### Scenario: arm64 host installs opencode-linux-arm64

- **WHEN** the image is built on an arm64 host
- **THEN** Dockerfile detects `dpkg --print-architecture == arm64` and downloads `opencode-linux-arm64.tar.gz`

#### Scenario: codex-acp ships via npm with platform-specific optional deps

- **WHEN** the image build runs `npm install -g @zed-industries/codex-acp`
- **THEN** npm resolves `optionalDependencies` for the host architecture (`linux-x64` on amd64, `linux-arm64` on arm64) and the `codex-acp` binary becomes available on `PATH`
- **AND** no extra `dpkg --print-architecture` branching is required for codex (npm handles it)

#### Scenario: All three runtime executables coexist in the image

- **WHEN** the image build completes
- **THEN** `/usr/local/bin/opencode`, the npm-installed `claude-agent-acp`, and the npm-installed `codex-acp` are all present and executable
- **AND** `AGENT_RUNTIME` at startup picks which one is used by the ACP path

#### Scenario: Unsupported architecture fails the build

- **WHEN** the image is built on a host whose `dpkg --print-architecture` is neither amd64 nor arm64
- **THEN** the Dockerfile RUN step exits non-zero with a clear error message (current behavior; not affected by codex addition)
