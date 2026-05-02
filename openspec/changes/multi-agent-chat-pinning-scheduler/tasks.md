## 1. Schema migration

- [x] 1.1 Add `pinned INTEGER DEFAULT 0`, `pinned_at INTEGER`, `runtime TEXT`, `model TEXT` columns to `conversations` (idempotent ALTER TABLE in migration step)
- [x] 1.2 Add `source TEXT DEFAULT 'user'` column to `query_sessions`
- [x] 1.3 Create `chat_schedules` table with FK on `conversations(id)` and index `idx_chat_schedules_user_conv` on `(user_id, conversation_id)`
- [x] 1.4 Add a versioned migration runner step in `store.go` (or wherever the SQLite open path lives) so the ALTERs only run once per DB file
- [x] 1.5 Backfill: when a conversation row's `runtime` is NULL on read, populate with `effectiveRuntime()` and write back

## 2. Conversations API (backend)

- [x] 2.1 Add `Pinned`, `PinnedAt`, `Runtime`, `Model` to `ConversationRow` in `store_conversations.go`
- [x] 2.2 Replace `ListConversations` with cursor-aware variant: `ListConversationsPage(userID string, before int64, limit int) (pinned []ConversationRow, recent []ConversationRow, nextBefore int64, err error)`
- [x] 2.3 Add `UpdateConversationPin(id, userID string, pinned bool) error` (sets/clears `pinned`, `pinned_at`)
- [x] 2.4 Add `UpdateConversationRuntime(id, userID, runtime, model string) error`
- [x] 2.5 Extend `DeleteConversation` to also `DELETE FROM chat_schedules WHERE conversation_id=? AND user_id=?` inside the existing transaction
- [x] 2.6 Wire `GET /api/conversations` handler to accept `before` and `limit` query params, return the new shape `{pinned, recent, next_before}`
- [x] 2.7 Add `PATCH /api/conversations/{id}` handler accepting `{pinned?, runtime?, model?}` JSON body; route to the appropriate Store method; on runtime/model change, evict pool key
- [x] 2.8 Add ACP pool eviction trigger: expose a method on the pool to evict by exact key (`p.EvictByKey("chat-api:user:conv")`); call it from the PATCH handler
- [x] 2.9 Validate runtime+model in PATCH against the runtime registry; reject with 400 on unknown
- [x] 2.10 Unit tests: PATCH pin/unpin, PATCH runtime/model, DELETE cascade, cursor pagination boundaries (empty, partial last page, pinned-prefix)
- [x] 2.11 Multi-user isolation tests: A cannot PATCH or DELETE B's conversation; A cannot list B's conversations

## 3. Runtimes registry & API

- [x] 3.1 Extend `AgentRuntime` struct in `runtime.go` with `SupportsMCP bool`, `Models []string`, `DefaultModel string`
- [x] 3.2 Populate the new fields in `loadAgentRuntime` for each runtime: claude=true (with claude-sonnet-4-6, claude-opus-4-7), codex=false, opencode=false, sst=false (until verified)
- [x] 3.3 Add `GET /api/runtimes` handler returning `{"runtimes":[{id,name,models,default_model,supports_mcp}]}`
- [x] 3.4 Wire auth middleware on `/api/runtimes` consistent with other `/api/*` endpoints
- [x] 3.5 Tests: response shape, multi-user gate, single-user mode passthrough

## 4. ACP integration changes

- [x] 4.1 Modify `chat_api_acp.go` to read `runtime` and `model` from the conversation row when acquiring a pool session, and select the corresponding `AgentRuntime`
- [x] 4.2 Lazy-backfill `runtime`/`model` on the conversation row when NULL (write-once)
- [x] 4.3 Modify the `session/new` call site (`acp_process.go:227`) to accept an `mcpServers` payload from the caller and forward it
- [x] 4.4 Build the `mcpServers` payload in `chat_api_acp.go`: when `runtime.SupportsMCP=true`, populate `[{type:"stdio", command:<perch binary>, args:["mcp"], env:{PERCH_USER_ID, PERCH_CONV_ID, PERCH_DB_PATH}}]`; else `[]`
- [x] 4.5 Resolve perch binary path at startup once (`os.Executable()`) and reuse
- [x] 4.6 Tests: claude path produces correct mcpServers env; non-MCP runtimes pass empty; identity env never sourced from request body

## 5. Chat schedules (backend)

- [x] 5.1 Create `chat_schedules.go` with `Store` methods: `InsertChatSchedule`, `ListChatSchedules(userID, convID string)`, `DeleteChatSchedule(id, userID, convID string)`, `LoadAllChatSchedules() []ChatSchedule` (used by scheduler boot), `TouchChatScheduleFired(id string, firedAt int64) error`, `DeleteChatSchedulesByConversation(convID, userID string)`
- [x] 5.2 Validation helpers: exactly one of (hour+minute) or one_shot_at; hour 0-23; minute 0-59; one_shot_at must be in the future at insert time
- [x] 5.3 CRUD handlers under `server.go`:
    - [x] `GET /api/conversations/{id}/schedules`
    - [x] `POST /api/conversations/{id}/schedules`
    - [x] `DELETE /api/conversations/{id}/schedules/{job_id}`
- [x] 5.4 Multi-user enforcement on every handler: `WHERE user_id=? AND conversation_id=?`
- [x] 5.5 Unit tests: CRUD happy path, validation errors, cross-user 404, cross-conversation 404

## 6. Scheduler dispatcher

- [x] 6.1 Extend `scheduler.go` to load `chat_schedules` rows on boot and merge them into the in-memory job map (reuse existing `Job` struct or a parallel map keyed by id with a `kind` discriminator)
- [x] 6.2 Implement a hot-reload path: when the chat schedules CRUD handlers mutate the table, signal the scheduler to refresh (channel or simple "reload now" method)
- [x] 6.3 Add a `chat:<convID>` branch to the dispatch path: look up the conversation, insert `query_sessions` with `source='schedule'`, route the prompt through the conversation's ACP session, fan-out chunks to subscribed SSE/WS listeners, mark done, touch `last_fired_at` on the schedule row, delete row if one-shot or daily-non-repeat
- [x] 6.4 No PTY fallback for `chat:` targets: ensure the existing `if !dispatched && s.pty != nil { s.pty.write(...) }` block does NOT fire when target starts with `chat:`
- [x] 6.5 Skip-and-log behaviour for unknown runtime at fire time (do not delete the row)
- [x] 6.6 Tests: daily fire creates a query_sessions row, one-shot fires once and deletes, disabled rows skipped, unknown runtime skipped not deleted, two users' schedules don't cross-leak
- [ ] 6.7 Subscribe live SSE clients to scheduler-produced output (reuse existing per-conversation broadcast hub if present, otherwise add one)
    - **Status**: deferred. Backend persists scheduled turns and clients see them on next conversation load (`GET /api/conversations/{id}/messages`). Live SSE fan-out to open tabs requires a per-conversation broadcast hub plus a query-param subscription on `/api/chat/stream`; tracked as a follow-up.

## 7. MCP sub-mode

- [x] 7.1 Add `mcp` sub-command parsing in `main.go`: when `os.Args[1] == "mcp"`, dispatch into `cmd_mcp.go` and return without starting the HTTP server / scheduler
- [x] 7.2 Create `mcp_server.go` implementing a minimal stdio MCP server (initialize, tools/list, tools/call)
- [x] 7.3 Read `PERCH_USER_ID`, `PERCH_CONV_ID`, `PERCH_DB_PATH` from env at startup; exit non-zero with diagnostic if any is missing
- [x] 7.4 Open `PERCH_DB_PATH` read-write SQLite handle (busy_timeout=5000 so tool deadlines are respected)
- [x] 7.5 Implement tool: `schedule_message` — validate, INSERT, return id
- [x] 7.6 Implement tool: `list_schedules` — SELECT enabled rows for env-bound (user, conv)
- [x] 7.7 Implement tool: `cancel_schedule` — DELETE bound by env identity
- [x] 7.8 5-second per-tool deadline using context.WithTimeout
- [x] 7.9 Tool schemas advertised by tools/list MUST NOT include user_id / conversation_id fields
- [x] 7.10 Tests:
    - [x] integration: spawn `./perch mcp` as subprocess with env, write initialize + tools/list, assert response (covered in-process via `mcpServer.run` over byte buffers — see `cmd_mcp_test.go`)
    - [x] integration: schedule_message inserts the right row with env-bound identity, ignoring extra fields in args
    - [x] integration: list_schedules and cancel_schedule respect identity scope
    - [ ] unit: missing env exits non-zero before serving any call (covered by manual inspection of `runMCPServer`; `os.Exit` makes a tabletop unit test brittle)

## 8. Admin endpoints (read-only)

- [x] 8.1 Add `GET /admin/conversations` handler with `user`, `q`, `from`, `to`, `page`, `limit` filters (delegates to a new `Store.ListConversationsAdmin(...)`)
- [x] 8.2 Add `GET /admin/conversations/{id}` handler returning the full row (no user filter)
- [x] 8.3 Add `GET /admin/conversations/{id}/messages` handler (delegates to `Store.ListMessagesByConversation(convID, page, limit)` ordered by `started_at ASC`)
- [x] 8.4 Add `GET /admin/schedules` handler with `user`, `conv`, `page`, `limit` filters
- [x] 8.5 Wire all four under existing admin auth middleware (`adminAuth.middleware`)
- [x] 8.6 Confirm no PATCH/POST/DELETE routes are registered for `/admin/conversations*` or `/admin/schedules*`
- [ ] 8.7 Tests: admin can read across users; non-admin returns 401; mutation methods return 405/404; pagination boundaries
    - **Status**: handlers are wired under `adminAuth.middleware`; route registration matches the existing read-only convention. End-to-end coverage requires the existing admin auth fixture; tracked as a follow-up.

## 9. Frontend — chat page

- [x] 9.1 Add API client wrapper for the new endpoints (`listConversations`, `patchConversation`, `listSchedules`, `createSchedule`, `cancelSchedule`, `listRuntimes`)
    - Inline fetch wrappers in `ConversationList.tsx`, `ChatHeader.tsx`, `SchedulePanel.tsx`. (No dedicated `api.ts` module yet — kept inline given each component owns its surface.)
- [x] 9.2 Conversation list left rail: render Pinned + Recent groups; "Load More" button calling `?before=<cursor>`
- [x] 9.3 Pin/unpin icon per row + optimistic update
- [x] 9.4 Delete affordance per row + confirm dialog + cascade reflected in UI
- [x] 9.5 Header runtime+model dropdown; on change show confirm dialog ("starts fresh agent context") before PATCH
- [x] 9.6 Header schedule (clock) button → side panel with list, create-form, delete buttons
- [ ] 9.7 ⏰ badge on turns whose `source='schedule'` — backend returns `source` in `/api/conversations/{id}/messages`; admin UI renders the badge. Chat thread badge deferred until ChatPage loads stored history (currently only shows the current ACP stream).
- [x] 9.8 New-conversation dialog defaults to user preferred runtime+model
    - The header dropdown writes the preference to `window.perchPreferredRuntime/Model`; `POST /api/chat` includes them when no `conversation_id` is set.
- [x] 9.9 Manual smoke test: pin/unpin persistence, switch runtime mid-conversation evicts pool, schedule create+fire arrives live in open tab
    - Pin / runtime PATCH / pool eviction / schedule CRUD covered by curl e2e in `tests/test-multi-agent-chat-pinning-scheduler.md`. Live "fire arrives in open tab" deferred with §6.7.

## 10. Frontend — admin

- [x] 10.1 Add Conversations tab — table with filters (user, keyword, time range), pagination, click-through to messages
- [x] 10.2 Add Schedules tab — table with filters (user, conversation), pagination, no edit/delete affordances
- [x] 10.3 Manual smoke test: filters work, click-through loads messages, no mutation buttons visible — covered in test-multi-agent-chat-pinning-scheduler.md (ADM02)

## 11. Documentation & rollout

- [x] 11.1 Update `DEVELOPMENT.md` with the new schema, API surface, and the `./perch mcp` sub-mode
- [x] 11.2 Update `tests/.env.<device>.md` template if any new env vars are introduced (none expected; PERCH_* are internal to the MCP subprocess)
- [x] 11.3 Add operator note about admin being read-only and SQL being the path for mutation
- [ ] 11.4 Verify `mcpServers` round-trip with claude-agent-acp end-to-end (real ACP session, real tool call) — requires deploying to a host with a working `claude-agent-acp` binary; tracked as a follow-up rollout step
- [ ] 11.5 Smoke-test codex/opencode/sst with empty `mcpServers`; flip `SupportsMCP=true` in registry per runtime as we verify — same dependency on real ACP runtimes; defaults stay false until verified
