## Requirements

### Requirement: Hook event routing to user sessions

The `/hook` endpoint SHALL route OpenCode hook events to the matching `userSession` based on `session_uuid`.

#### Scenario: PreToolUse hook received
- **WHEN** a PreToolUse hook event arrives with a `session_uuid` that matches a known user session
- **THEN** the server SHALL emit a `tool_start` JSON message to that user's WebSocket subscribers:
  `{"type":"tool_start","tool":"<toolName>","input":<truncated_input>}`

#### Scenario: PostToolUse hook received
- **WHEN** a PostToolUse hook event arrives for a known session
- **THEN** the server SHALL emit a `tool_end` JSON message:
  `{"type":"tool_end","tool":"<toolName>","elapsed_ms":<N>}`

#### Scenario: Stop hook received
- **WHEN** a Stop hook event arrives for a known session
- **THEN** the server SHALL emit `{"type":"done"}` to all WebSocket subscribers, then mark the session as completed

#### Scenario: unknown session_uuid
- **WHEN** a hook event arrives with a `session_uuid` that does not match any user session
- **THEN** the server SHALL log a warning and discard the event

### Requirement: WebSocket message framing

The `/ws/chat` WebSocket SHALL multiplex two message types on the same connection, distinguished by content:
- **Binary messages**: raw PTY output bytes (rendered as markdown)
- **Text messages**: structured JSON events (`tool_start`, `tool_end`, `done`)

#### Scenario: client receives binary message
- **WHEN** a binary WebSocket message arrives
- **THEN** the frontend SHALL append the bytes to the markdown render buffer

#### Scenario: client receives text message
- **WHEN** a text WebSocket message arrives and parses as JSON with a `type` field
- **THEN** the frontend SHALL update the tool call panel accordingly
