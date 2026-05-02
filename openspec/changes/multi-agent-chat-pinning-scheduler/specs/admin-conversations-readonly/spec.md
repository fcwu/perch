## ADDED Requirements

### Requirement: GET /admin/conversations lists all conversations across users

`GET /admin/conversations?user=<id>&q=<keyword>&from=<ms>&to=<ms>&page=<n>&limit=<n>` SHALL return a paginated list of conversations across all users, gated by the existing admin authentication middleware. Filters: `user` (exact match on `user_id`), `q` (case-insensitive substring on `title`), `from`/`to` (range on `updated_at`). Pagination uses `page` (1-based) and `limit` (default 20, max 100). The response includes `user_id`, `runtime`, and `model` for each row alongside the standard conversation fields.

#### Scenario: Admin lists all conversations

- **WHEN** an admin calls `GET /admin/conversations?page=1&limit=20`
- **THEN** the response is `{"total":N,"conversations":[{"id":"...","user_id":"...","title":"...","runtime":"...","model":"...","pinned":0|1,"created_at":...,"updated_at":...}, ...]}`

#### Scenario: Admin filters by user

- **WHEN** an admin calls `GET /admin/conversations?user=alice@example.com`
- **THEN** only rows where `user_id="alice@example.com"` are returned

#### Scenario: Admin filters by keyword

- **WHEN** an admin calls `GET /admin/conversations?q=migration`
- **THEN** only rows whose `title` contains "migration" (case-insensitive) are returned

#### Scenario: Non-admin gets 401

- **WHEN** a request without a valid admin cookie hits `GET /admin/conversations`
- **THEN** the server returns 401 (or redirects per the existing admin middleware)

### Requirement: GET /admin/conversations/{id} returns one conversation's metadata

`GET /admin/conversations/{id}` SHALL return the full conversation row regardless of which user owns it.

#### Scenario: Admin reads any conversation row

- **WHEN** an admin calls `GET /admin/conversations/<any-id>`
- **THEN** the response includes the full row: `id`, `user_id`, `title`, `runtime`, `model`, `pinned`, `pinned_at`, `created_at`, `updated_at`
- **AND** if the id does not exist, the server returns 404

### Requirement: GET /admin/conversations/{id}/messages returns the conversation's turns

`GET /admin/conversations/{id}/messages?page=<n>&limit=<n>` SHALL return the `query_sessions` rows belonging to that conversation, ordered by `started_at ASC`, including `id, user_id, query, response, status, started_at, ended_at, source`.

#### Scenario: Admin reads conversation turns

- **WHEN** an admin calls `GET /admin/conversations/<id>/messages`
- **THEN** the response is `{"total":N, "messages":[{"id":"...","query":"...","response":"...","status":"done","source":"user"|"schedule","started_at":...,"ended_at":...}, ...]}`

#### Scenario: Source flag is preserved

- **WHEN** the conversation has both manually-typed turns and scheduler-fired turns
- **THEN** the `source` field on each row reflects the original insertion (`'user'` or `'schedule'`)

### Requirement: GET /admin/schedules lists all schedules

`GET /admin/schedules?user=<id>&conv=<id>&page=<n>&limit=<n>` SHALL return a paginated list of `chat_schedules` rows across all users, with optional filters by `user_id` and `conversation_id`.

#### Scenario: Admin lists all schedules

- **WHEN** an admin calls `GET /admin/schedules?page=1&limit=20`
- **THEN** the response is `{"total":N,"schedules":[{"id":"...","user_id":"...","conversation_id":"...","hour":...,"minute":...,"repeat":...,"one_shot_at":...,"prompt":"...","enabled":1,"created_at":...,"last_fired_at":...}, ...]}`

#### Scenario: Admin filters schedules by user

- **WHEN** an admin calls `GET /admin/schedules?user=alice@example.com`
- **THEN** only rows where `user_id="alice@example.com"` are returned

### Requirement: Admin endpoints are read-only

The admin surface SHALL NOT expose mutation endpoints for conversations or schedules (no PATCH, POST, DELETE on `/admin/conversations*` or `/admin/schedules*`). Operators wishing to mutate user data SHALL use direct SQL.

#### Scenario: PATCH on admin conversation is rejected

- **WHEN** an admin sends `PATCH /admin/conversations/<id>` or `DELETE /admin/conversations/<id>`
- **THEN** the server returns 405 Method Not Allowed (or 404 if the route is simply not registered)

#### Scenario: POST on admin schedule is rejected

- **WHEN** an admin sends `POST /admin/schedules` or `DELETE /admin/schedules/<id>`
- **THEN** the server returns 405 Method Not Allowed (or 404 if the route is simply not registered)

### Requirement: Admin UI gains Conversations and Schedules tabs

The frontend admin SPA SHALL include two new tabs alongside History: "Conversations" (list with filters and a click-through to messages view) and "Schedules" (list with user/conversation filters). Both tabs SHALL be read-only — no edit, no delete buttons.

#### Scenario: Conversations tab shows global list

- **WHEN** an admin loads the Conversations tab
- **THEN** the page calls `GET /admin/conversations` and renders the rows in a table with columns: user, title, runtime, model, updated_at
- **AND** clicking a row navigates to a messages view that calls `GET /admin/conversations/<id>/messages`

#### Scenario: Schedules tab shows global list

- **WHEN** an admin loads the Schedules tab
- **THEN** the page calls `GET /admin/schedules` and renders rows in a table with columns: user, conversation, time/one-shot, prompt, enabled, last_fired_at
- **AND** the row UI carries no edit, pause, or delete affordances
