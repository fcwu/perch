## ADDED Requirements

### Requirement: Admin real-time session feed

`GET /ws/admin` SHALL stream live session state changes to authenticated admin clients.

#### Scenario: initial connection snapshot
- **WHEN** an admin WebSocket client connects
- **THEN** the server SHALL immediately push a `session_snapshot` message containing all currently active sessions:
  `{"type":"session_snapshot","sessions":[{"id":"...","username":"...","query":"...","status":"running","current_tool":"read","started_at":...}]}`

#### Scenario: new session starts
- **WHEN** a user starts a new query session
- **THEN** the server SHALL push `{"type":"session_added","session":{...}}` to all connected admin clients

#### Scenario: tool state changes
- **WHEN** a tool_start or tool_end event occurs in any user session
- **THEN** the server SHALL push `{"type":"session_update","id":"<session_uuid>","current_tool":"<name or null>"}` to all admin clients

#### Scenario: session ends
- **WHEN** a user session completes or times out
- **THEN** the server SHALL push `{"type":"session_removed","id":"<session_uuid>","status":"done|timeout"}` to all admin clients

### Requirement: Admin real-time UI

The `/admin` page SHALL display a live list of active sessions, updating without page refresh.

#### Scenario: session list renders
- **WHEN** the admin page loads and the WebSocket connects
- **THEN** the UI SHALL display each active session as a row: username, truncated query, elapsed time, current tool (or idle), status indicator

#### Scenario: session row updates
- **WHEN** a `session_update` event arrives
- **THEN** the corresponding row SHALL update the current tool field in place without re-rendering the whole list
