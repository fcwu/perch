> Note: this capability supersedes the prior `admin-realtime` capability (renamed). All Requirements from `admin-realtime` move here verbatim except where MODIFIED below; the source-of-events change (hook → ACP) is captured in `acp-tool-events`. The capability directory at `openspec/specs/admin-realtime/` is removed and its content moves to `openspec/specs/management-realtime/`.

## ADDED Requirements

### Requirement: Live management endpoint is gated by multi-user mode

The Live management WebSocket endpoint (`/ws/management`, formerly `/ws/admin`) SHALL be available only when the perch instance is running in multi-user mode (`PERCH_MODE=multi`). In single-user mode the endpoint SHALL NOT be registered, and requests SHALL receive HTTP 404.

#### Scenario: Multi-user mode registers the endpoint

- **WHEN** the container starts with `PERCH_MODE=multi` (and the corresponding auth providers configured)
- **THEN** `server.go` registers `/ws/management` with `managementMW`
- **AND** authenticated admin clients can subscribe and receive snapshot + delta events

#### Scenario: Single-user mode does not register the endpoint

- **WHEN** the container starts with `PERCH_MODE=single` (the default)
- **THEN** `server.go` does NOT register `/ws/management`
- **AND** any HTTP/WebSocket request to `/ws/management` returns 404

#### Scenario: Frontend hides Live tab in single-user mode

- **WHEN** the management page (`/management`) loads in single-user mode
- **THEN** the Live tab is not shown (or is shown disabled with explanatory tooltip)
- **AND** the History tab remains visible (history works in all modes)

## MODIFIED Requirements

### Requirement: Live management WebSocket pushes session snapshot and delta events

When an authenticated admin connects to `/ws/management` (in multi-user mode), perch SHALL send a `session_snapshot` event listing all active query sessions, then push `session_added`, `session_update`, and `session_removed` events as `ManagementHub` state changes.

> Modification rationale: route renamed (`/ws/admin` → `/ws/management`); event source now ACP-driven (see `acp-tool-events`); access scope is multi-user-only (see ADDED Requirement above).

#### Scenario: Initial snapshot

- **WHEN** an admin client subscribes to `/ws/management`
- **THEN** the first message is `{type: "session_snapshot", sessions: [...]}` listing all sessions currently in `ManagementHub`

#### Scenario: New chat-API session appears

- **WHEN** a new chat-API ACP prompt starts
- **THEN** subscribed clients receive `{type: "session_added", session: {...}}`

#### Scenario: Tool change updates current_tool

- **WHEN** ACP emits `tool_call_started` for a tracked session
- **THEN** subscribed clients receive `{type: "session_update", session: {id: "...", current_tool: "Bash", ...}}`

#### Scenario: Session ends

- **WHEN** ACP emits `RunCompleted` or `RunFailed` for a tracked session
- **THEN** subscribed clients receive `{type: "session_removed", id: "...", status: "done"|"error"}`
