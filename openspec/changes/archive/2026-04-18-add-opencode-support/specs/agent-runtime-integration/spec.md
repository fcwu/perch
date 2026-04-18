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
The runtime image SHALL include the executable and bundled support files required by the selected runtime.

#### Scenario: Claude image support preserved
- **WHEN** the container image is built for the existing default runtime path
- **THEN** the image still contains the Claude executable and bundled Claude support assets required by Perch

#### Scenario: OpenCode runtime is available in image
- **WHEN** the container starts with the active runtime set to `opencode`
- **THEN** the image already contains the OpenCode executable and bundled OpenCode support assets needed for startup
