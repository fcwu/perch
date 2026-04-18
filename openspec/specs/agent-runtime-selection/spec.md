## ADDED Requirements

### Requirement: Configurable active agent runtime
The system SHALL allow Perch to select a single active agent runtime by configuration, with `claude` as the default when no runtime is explicitly set.

#### Scenario: Default runtime remains Claude
- **WHEN** Perch starts without an explicit runtime selection
- **THEN** the main PTY starts the Claude runtime and existing Claude-based behavior remains unchanged

#### Scenario: OpenCode runtime selected
- **WHEN** Perch starts with the runtime configured as `opencode`
- **THEN** the main PTY starts the OpenCode runtime instead of Claude

#### Scenario: Invalid runtime rejected
- **WHEN** Perch starts with an unsupported runtime value
- **THEN** startup fails with a clear configuration error instead of silently falling back to another runtime

### Requirement: All PTY entry points use the active runtime
The system MUST use the same active agent runtime for the main PTY, Discord session PTYs, and scheduler-triggered PTYs.

#### Scenario: Discord session uses OpenCode
- **WHEN** the active runtime is `opencode` and a Discord message creates or reuses a Discord PTY session
- **THEN** that PTY session starts OpenCode rather than Claude

#### Scenario: Scheduler target uses selected runtime
- **WHEN** a scheduler job writes to the main PTY or a Discord PTY target
- **THEN** the target PTY is backed by the currently selected runtime rather than a hard-coded agent command

### Requirement: Runtime-specific CLI arguments are isolated by runtime
The system SHALL apply runtime-specific extra CLI arguments only to the runtime they belong to.

#### Scenario: Claude args do not leak into OpenCode
- **WHEN** the active runtime is `opencode`
- **THEN** Claude-specific CLI argument configuration is not appended to the OpenCode command line

#### Scenario: OpenCode args do not affect Claude
- **WHEN** the active runtime is `claude`
- **THEN** OpenCode-specific CLI argument configuration does not affect Claude startup
