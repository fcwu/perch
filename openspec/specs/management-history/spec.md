## Requirements

### Requirement: History list API

`GET /api/management/history` SHALL return a paginated list of past query sessions from the log store.

#### Scenario: list all sessions
- **WHEN** management calls `GET /api/management/history?page=1&limit=20`
- **THEN** the server SHALL return JSON: `{"total":N,"sessions":[{"id":"...","username":"...","query":"...","status":"done","started_at":...,"ended_at":...,"duration_ms":...}]}`
- Response SHALL NOT include `response` or `tool_events` (kept for detail endpoint)

#### Scenario: filter by user
- **WHEN** management calls `GET /api/management/history?user=alice`
- **THEN** only sessions where `username = "alice"` SHALL be returned

#### Scenario: filter by time range
- **WHEN** management calls `GET /api/management/history?from=<unix_ms>&to=<unix_ms>`
- **THEN** only sessions whose `started_at` falls within the range SHALL be returned

#### Scenario: keyword search
- **WHEN** management calls `GET /api/management/history?q=kubernetes`
- **THEN** only sessions where `query` contains the keyword (case-insensitive, SQLite LIKE) SHALL be returned

### Requirement: Session detail API

`GET /api/management/history/<session_id>` SHALL return the full detail of one session including tool events and response.

#### Scenario: session found
- **WHEN** management calls `GET /api/management/history/<valid_id>`
- **THEN** the server SHALL return the full session row plus `tool_events` array in chronological order and the `response` text

#### Scenario: session not found
- **WHEN** `<session_id>` does not exist in the store
- **THEN** the server SHALL return HTTP 404

### Requirement: History UI

The `/management/history` page SHALL display the session list with search controls and allow drilling into a single session.

#### Scenario: search and filter
- **WHEN** management enters text in the search box or selects a user filter
- **THEN** the UI SHALL debounce and call the history API, updating the list in place

#### Scenario: session detail view
- **WHEN** management clicks a row
- **THEN** the UI SHALL expand or navigate to a detail view showing the full query, response, and tool call timeline
