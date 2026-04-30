> Note: this capability supersedes the prior `admin-history` capability (renamed). All Requirements from `admin-history` move here verbatim except the URL prefix is `/api/management/history` instead of `/admin/history`. The capability directory at `openspec/specs/admin-history/` is removed and its content moves to `openspec/specs/management-history/`. The write-trigger source change (Stop hook → ACP `RunCompleted`) is captured in `acp-tool-events`.

## MODIFIED Requirements

### Requirement: History list API

`GET /api/management/history` SHALL return a paginated list of past query sessions from the log store. (URL prefix renamed from `/admin/history` to `/api/management/history`; behavior otherwise unchanged.)

#### Scenario: list all sessions

- **WHEN** management calls `GET /api/management/history?page=1&limit=20`
- **THEN** the server SHALL return JSON: `{"total":N,"sessions":[{"id":"...","username":"...","query":"...","status":"done","started_at":...,"ended_at":...,"duration_ms":...}]}`

### Requirement: Session detail API

`GET /api/management/history/<session_id>` SHALL return the full detail of one session. (URL prefix renamed; behavior otherwise unchanged.)

#### Scenario: session found

- **WHEN** management calls `GET /api/management/history/<valid_id>`
- **THEN** the server SHALL return the full session row plus `tool_events` array in chronological order and the `response` text

### Requirement: History UI

The `/management/history` page SHALL display the session list with search controls and allow drilling into a single session. (Route renamed from `/admin/history` to `/management/history`.)

#### Scenario: session detail view

- **WHEN** management clicks a row
- **THEN** the UI SHALL expand or navigate to a detail view showing the full query, response, and tool call timeline
