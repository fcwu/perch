## Requirements

### Requirement: Live management endpoint is gated by multi-user mode

The Live management WebSocket endpoint (`/ws/management`) SHALL be available only when the perch instance is running in multi-user mode (`PERCH_MODE=multi`). In single-user mode the endpoint SHALL NOT be registered, and requests SHALL receive HTTP 404.

#### Scenario: Multi-user mode registers the endpoint

- **WHEN** the container starts with `PERCH_MODE=multi` (and the corresponding auth providers configured)
- **THEN** `server.go` registers `/ws/management` with `managementMW`
- **AND** authenticated management clients can subscribe and receive snapshot + delta events

#### Scenario: Single-user mode does not register the endpoint

- **WHEN** the container starts with `PERCH_MODE=single` (the default)
- **THEN** `server.go` registers `/ws/management` with an explicit `http.NotFound` handler so the path does not fall through to the SPA
- **AND** any HTTP/WebSocket request to `/ws/management` returns 404

#### Scenario: Frontend hides Live tab in single-user mode

- **WHEN** the management page (`/management`) loads in single-user mode
- **THEN** the Live tab is not shown (or is shown disabled with explanatory tooltip)
- **AND** the History tab remains visible (history works in all modes)

### Requirement: Management real-time session feed

`GET /ws/management` SHALL stream live session state changes to authenticated management clients.

#### Scenario: initial connection snapshot
- **WHEN** a management WebSocket client connects
- **THEN** the server SHALL immediately push a `session_snapshot` message containing all currently active sessions:
  `{"type":"session_snapshot","sessions":[{"id":"...","username":"...","query":"...","status":"running","current_tool":"read","started_at":...}]}`

#### Scenario: new session starts
- **WHEN** a user starts a new query session
- **THEN** the server SHALL push `{"type":"session_added","session":{...}}` to all connected management clients

#### Scenario: tool state changes
- **WHEN** a tool_start or tool_end event occurs in any user session
- **THEN** the server SHALL push `{"type":"session_update","id":"<session_uuid>","current_tool":"<name or null>"}` to all management clients

#### Scenario: session ends
- **WHEN** a user session completes or times out
- **THEN** the server SHALL push `{"type":"session_removed","id":"<session_uuid>","status":"done|timeout"}` to all management clients

### Requirement: Management real-time UI

The `/management` page SHALL display a live list of active sessions, updating without page refresh.

#### Scenario: session list renders
- **WHEN** the management page loads and the WebSocket connects
- **THEN** the UI SHALL display each active session as a row: username, truncated query, elapsed time, current tool (or idle), status indicator

#### Scenario: session row updates
- **WHEN** a `session_update` event arrives
- **THEN** the corresponding row SHALL update the current tool field in place without re-rendering the whole list
