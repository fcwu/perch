# chat-conversations Specification

## Purpose

Per-user conversation rows, per-conversation runtime/model selection, pinning, cursor pagination, and cascading delete. Owns the `conversations` table and `/api/conversations*` endpoints.

## Requirements

### Requirement: Conversation rows carry pinning, runtime, and model state

The `conversations` table SHALL store, per row, a `pinned` boolean (0/1), a `pinned_at` epoch-ms timestamp, a `runtime` short string identifier (e.g., `claude`, `codex`, `opencode`, `sst`), and a `model` short string identifier (e.g., `claude-sonnet-4-6`, `gpt-5-codex`). New conversations SHALL inherit `runtime` and `model` from the user's preference (or the server default if unset). `pinned` defaults to 0; `pinned_at` is NULL when unpinned.

#### Scenario: New conversation gets default runtime and model

- **WHEN** a user submits the first message of a new conversation and has not customised their preferences
- **THEN** the inserted `conversations` row has `runtime` equal to the server's default runtime and `model` equal to that runtime's default model
- **AND** `pinned=0` and `pinned_at` is NULL

#### Scenario: New conversation honours user preference

- **WHEN** the user has set a preferred runtime+model in their settings and submits the first message of a new conversation
- **THEN** the inserted row uses that runtime+model

### Requirement: Pin and unpin via PATCH /api/conversations/{id}

`PATCH /api/conversations/{id}` SHALL accept `{"pinned": true|false}` and SHALL toggle the corresponding row, scoped to the authenticated user. When toggling to `true`, `pinned_at` SHALL be set to the current time in epoch ms; when toggling to `false`, `pinned_at` SHALL be cleared.

#### Scenario: Owner pins a conversation

- **WHEN** the authenticated owner sends `PATCH /api/conversations/<id> {"pinned":true}`
- **THEN** the row's `pinned=1` and `pinned_at` is set to now-ms
- **AND** the response is 200 with the updated row

#### Scenario: Non-owner cannot pin another user's conversation

- **WHEN** user A sends `PATCH /api/conversations/<B-conv-id> {"pinned":true}`
- **THEN** the server returns 404 (treated as not-found) and the row is unchanged

#### Scenario: Single-user mode pin works against the synthetic default user

- **WHEN** single-user mode is enabled and a request hits `PATCH /api/conversations/<id> {"pinned":true}`
- **THEN** the server resolves the user as `"default"` and the row owned by `"default"` is updated

### Requirement: Set runtime and/or model via PATCH /api/conversations/{id}

`PATCH /api/conversations/{id}` SHALL accept `{"runtime": "<id>", "model": "<id>"}` (either field optional) and update the conversation row. Whenever `runtime` or `model` changes, the server SHALL evict the corresponding ACP pool key (`chat-api:<user>:<conv>`) so that the next prompt boots a fresh ACP subprocess against the new runtime.

#### Scenario: Switch runtime mid-conversation

- **WHEN** the owner sends `PATCH /api/conversations/<id> {"runtime":"opencode","model":"opencode-default"}`
- **THEN** the row's `runtime` and `model` are updated
- **AND** the ACP session pool entry for `(user, conv)` is evicted
- **AND** the next user message in this conversation spawns a new ACP subprocess for `opencode`

#### Scenario: Unsupported runtime or model is rejected

- **WHEN** the request contains a runtime not in the runtime registry, or a model not advertised by `GET /api/runtimes` for the chosen runtime
- **THEN** the server returns 400 and does not modify the row

### Requirement: Conversation list uses cursor pagination with pinned-prefix

`GET /api/conversations?before=<updated_at_ms>&limit=<n>` SHALL return the authenticated user's conversations. When `before=0` or omitted (i.e., the first page), the response SHALL include all pinned rows ordered by `pinned_at DESC` followed by up to `limit` non-pinned rows ordered by `updated_at DESC`. When `before>0`, only non-pinned rows with `updated_at < before` SHALL be returned, up to `limit` rows. The default `limit` is 20 and the maximum is 100.

#### Scenario: First page returns pinned rows then recent

- **WHEN** the user has 3 pinned and 50 non-pinned conversations and requests `GET /api/conversations?limit=20`
- **THEN** the response is `{ "pinned": [3 rows ordered by pinned_at DESC], "recent": [20 non-pinned rows ordered by updated_at DESC], "next_before": <updated_at of last recent row> }`

#### Scenario: Subsequent page omits pinned rows

- **WHEN** the user requests `GET /api/conversations?before=<ms>&limit=20` with `before > 0`
- **THEN** the response is `{ "recent": [up to 20 rows where pinned=0 AND updated_at < before, ordered DESC], "next_before": <updated_at of last row> }` and `pinned` is omitted or empty

#### Scenario: Last page sets next_before to 0 or null

- **WHEN** the page returns fewer rows than `limit`
- **THEN** `next_before` is omitted, null, or 0 to signal end-of-list

### Requirement: Conversation deletion cascades to chat_schedules

When `DELETE /api/conversations/{id}` succeeds, the server SHALL also delete all rows in `chat_schedules` whose `conversation_id` matches, scoped to the same user.

#### Scenario: Delete removes schedules

- **WHEN** the owner deletes a conversation that has 2 active schedules
- **THEN** the conversation row, its `query_sessions` rows, and the 2 `chat_schedules` rows are removed in a single transaction

#### Scenario: Delete is idempotent on already-empty schedules

- **WHEN** the owner deletes a conversation with no schedules
- **THEN** the conversation row and `query_sessions` rows are removed and the operation succeeds with 204

### Requirement: Scheduler-fired turns are tagged on query_sessions

The `query_sessions` table SHALL carry a `source TEXT DEFAULT 'user'` column. Manual messages from `POST /api/chat` SHALL insert with `source='user'`; messages produced by the scheduler dispatcher SHALL insert with `source='schedule'`. Conversation message-list endpoints SHALL include `source` in their response shape.

#### Scenario: Manual message defaults to user source

- **WHEN** the user types a message in chat
- **THEN** the inserted `query_sessions` row has `source='user'`

#### Scenario: Scheduled fire records schedule source

- **WHEN** the scheduler dispatches a `chat:<convID>` job
- **THEN** the inserted `query_sessions` row has `source='schedule'`

### Requirement: GET /api/runtimes lists available runtime+model combinations

`GET /api/runtimes` SHALL return the runtimes the server can spawn, including for each runtime the runtime id, display name, list of available model ids, default model id, and a `supports_mcp` boolean.

#### Scenario: Default response shape

- **WHEN** an authenticated user requests `GET /api/runtimes`
- **THEN** the response is `{"runtimes":[{"id":"claude","name":"Claude","models":["claude-sonnet-4-6","claude-opus-4-7"],"default_model":"claude-sonnet-4-6","supports_mcp":true}, ...]}`

#### Scenario: supports_mcp drives agent-tool affordances in the UI

- **WHEN** the runtime entry has `supports_mcp=false`
- **THEN** the UI MAY surface a footnote that the agent cannot self-schedule for this runtime, but the UI scheduler button remains available
