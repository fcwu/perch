## ADDED Requirements

### Requirement: Query session persistence

The log store SHALL persist each query session to SQLite with full lifecycle data.

#### Scenario: session starts
- **WHEN** `UserSessionManager` starts a new session
- **THEN** the store SHALL insert a row into `query_sessions` with status `"running"`, recording `session_uuid`, `user_id`, `username`, `query`, and `started_at`

#### Scenario: session completes
- **WHEN** a Stop hook event arrives for a session
- **THEN** the store SHALL update the row: set `response` (extracted assistant text), `ended_at`, and `status = "done"`

#### Scenario: session times out
- **WHEN** a session is terminated due to timeout
- **THEN** the store SHALL update `status = "timeout"` and `ended_at`

### Requirement: Tool event persistence

Each tool call SHALL be recorded in the `tool_events` table.

#### Scenario: tool starts
- **WHEN** a PreToolUse hook event is received for a known session
- **THEN** the store SHALL insert a row into `tool_events` with `tool_name`, `input_json`, and `started_at`

#### Scenario: tool completes
- **WHEN** a PostToolUse hook event is received
- **THEN** the store SHALL update the matching `tool_events` row with `output_json` and `ended_at`

### Requirement: WAL mode enabled

The SQLite database SHALL be opened with WAL (Write-Ahead Logging) mode to allow concurrent reads during writes.

#### Scenario: concurrent sessions writing
- **WHEN** multiple sessions write tool events simultaneously
- **THEN** the database SHALL not deadlock; all writes SHALL succeed
