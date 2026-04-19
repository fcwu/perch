## ADDED Requirements

### Requirement: JSON structured log output

When `LOG_FORMAT=json`, the server SHALL use `slog.NewJSONHandler` and emit all query lifecycle events with standardized fields.

#### Scenario: query starts
- **WHEN** a user session starts
- **THEN** the server SHALL log `{"msg":"query_start","user_id":"...","username":"...","session_id":"...","query":"..."}`

#### Scenario: tool executes
- **WHEN** a PreToolUse hook is received
- **THEN** the server SHALL log `{"msg":"tool_start","session_id":"...","tool":"<name>"}`

#### Scenario: query completes
- **WHEN** a session ends (Stop hook or timeout)
- **THEN** the server SHALL log `{"msg":"query_done","session_id":"...","duration_ms":<N>,"tool_count":<N>,"status":"done|timeout"}`

### Requirement: LOG_FORMAT environment variable

`LOG_FORMAT` SHALL accept `text` (default) or `json`.

#### Scenario: LOG_FORMAT unset or text
- **WHEN** `LOG_FORMAT` is empty or `"text"`
- **THEN** the server SHALL use the existing text slog handler (no change in behavior)

#### Scenario: LOG_FORMAT=json
- **WHEN** `LOG_FORMAT=json`
- **THEN** all log output SHALL be newline-delimited JSON to stdout
