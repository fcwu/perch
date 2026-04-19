## ADDED Requirements

### Requirement: SessionProvider write support

The `SessionProvider` interface SHALL include a `WriteSession(channelID string, data []byte) error` method. Any adapter implementing `SessionProvider` MUST implement `WriteSession`.

#### Scenario: write to existing session
- **WHEN** `WriteSession` is called with a channelID that has an active PTY
- **THEN** the data SHALL be written to that PTY's stdin

#### Scenario: write to non-existent session
- **WHEN** `WriteSession` is called with a channelID that has no active PTY
- **THEN** the method SHALL return an error; the caller MUST handle it gracefully (e.g., log and discard)

### Requirement: WebSocket session input forwarding

The `/ws/session` WebSocket endpoint SHALL forward non-resize messages to the PTY as keystrokes.

#### Scenario: keystroke forwarded to PTY
- **WHEN** a WebSocket client sends a binary or text message that is not a valid resize JSON
- **THEN** the server SHALL call `WriteSession` with the raw message bytes

#### Scenario: resize still works
- **WHEN** a WebSocket client sends `{"type":"resize","cols":N,"rows":N}`
- **THEN** the server SHALL call `ResizeSession` and NOT write the message to the PTY

#### Scenario: write error is non-fatal
- **WHEN** `WriteSession` returns an error (e.g., session closed)
- **THEN** the server SHALL log the error and continue; the WebSocket connection SHALL remain open
