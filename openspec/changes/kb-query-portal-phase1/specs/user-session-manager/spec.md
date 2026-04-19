## ADDED Requirements

### Requirement: Per-user OpenCode PTY session

The `UserSessionManager` SHALL maintain a map of `userID → *userSession`, where each session contains an independent OpenCode PTY running the `as-query` subagent.

#### Scenario: user sends first query
- **WHEN** an authenticated user submits a query and no session exists for their `userID`
- **THEN** the server SHALL create a new `userSession`, start an OpenCode PTY with `opencode run --agent as-query "<query>"`, and stream PTY output to the user's WebSocket

#### Scenario: session already active
- **WHEN** an authenticated user submits a query while their previous session is still running
- **THEN** the server SHALL return HTTP 409 and inform the user that a session is already in progress

#### Scenario: session completes
- **WHEN** the OpenCode process exits (Stop hook received or PTY EOF)
- **THEN** the session SHALL be marked as completed; the PTY output SHALL be retained for replay for 5 minutes, then the session SHALL be cleaned up

#### Scenario: session timeout
- **WHEN** an OpenCode session has been running for more than 5 minutes without a Stop event
- **THEN** the server SHALL terminate the PTY and clean up the session

### Requirement: Session subscription

`UserSessionManager` SHALL implement the `SessionProvider` interface, allowing the server to subscribe to PTY output and push it to WebSocket clients.

#### Scenario: WebSocket subscribes to session output
- **WHEN** a client connects to `/ws/chat?id=<userID>`
- **THEN** the server SHALL subscribe to that user's PTY output channel and forward all bytes to the WebSocket

#### Scenario: multiple WebSocket connections for same user
- **WHEN** the same user opens multiple browser tabs
- **THEN** each WebSocket connection SHALL receive the same PTY output (fan-out)

### Requirement: session_uuid to userID mapping

When a PTY session starts, `UserSessionManager` SHALL record the `session_uuid` (from the first hook event) in a `uuid → userID` lookup map, used to route hook events back to the correct user session.

#### Scenario: hook event arrives with session_uuid
- **WHEN** a PreToolUse/PostToolUse/Stop hook event arrives at `/hook` containing a known `session_uuid`
- **THEN** the server SHALL route the event to the matching `userSession`
