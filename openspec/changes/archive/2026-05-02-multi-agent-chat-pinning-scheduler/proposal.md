## Why

Today perch's web chat treats conversations as ephemeral: history is reconstructed from a sliding 24-hour window of `query_sessions`, the conversation list is hard-capped at 50 rows, every conversation is locked to whichever runtime was active at session start, and there is no way to pin an important thread, change agents mid-conversation, or schedule a follow-up message. The product is now a **multi-agent assistant** (claude / codex / opencode / sst), and users want to keep work threads alive indefinitely, switch agents per task, and have agents/users schedule reminders that fire back into the originating thread without leaking across users.

We also lack admin visibility into per-conversation state — `/admin/history` shows raw `query_sessions` rows but no conversation-level grouping or scheduler view, which makes operational triage hard once retention becomes infinite.

## What Changes

**Conversations**
- **BREAKING**: Remove the 24-hour history window in favour of indefinite retention; conversation history is now read directly from the `conversations` + `query_sessions` tables until the user deletes it.
- Add **pinning**: a `pinned`/`pinned_at` flag returned in a separate top group that never falls off the list.
- Add **per-conversation runtime + model**: each conversation stores its own `runtime` (claude/codex/opencode/sst) and `model`; switching mid-conversation keeps UI history but starts a fresh ACP session.
- Replace the `LIMIT 50` list with **cursor-based "Load More"** (`?before=<updated_at_ms>&limit=20`); pinned rows are always returned at the top regardless of cursor.
- Cascade-delete `chat_schedules` rows when a conversation is deleted.
- Tag scheduler-fired turns in `query_sessions.source = 'schedule'` so the UI can render a ⏰ badge.

**Per-conversation scheduler**
- New `chat_schedules` table (user_id + conversation_id scoped) with daily H/M+repeat **or** one-shot epoch-ms triggers.
- New CRUD: `GET/POST/DELETE /api/conversations/{id}/schedules`.
- A fired schedule injects its prompt into the conversation as if the user typed it, routed through that conversation's runtime+model; reply lands in the same conversation and is delivered via existing SSE.
- UI button on the chat header for users to add/list/cancel schedules.

**MCP agent tool (perch self-hosted)**
- New `./perch mcp` sub-mode runs an stdio MCP server.
- ACP `session/new` is changed to launch this server per session via `mcpServers`, threading `PERCH_USER_ID` / `PERCH_CONV_ID` / `PERCH_DB_PATH` through env. Identity is **never** trusted from agent input.
- Three tools: `schedule_message`, `list_schedules`, `cancel_schedule` — all scoped to the env-locked (user, conversation) tuple.
- Runtimes that don't honor `mcpServers` degrade gracefully: agent loses the tool, UI button still works.

**Multi-user isolation**
- Every new endpoint (PATCH conversation, schedule CRUD, cursor list, runtimes list) enforces `WHERE user_id=?`.
- Single-user mode keeps using the synthetic `"default"` user_id; behaviour is identical.

**Admin (read-only)**
- New `GET /admin/conversations`, `/admin/conversations/{id}`, `/admin/conversations/{id}/messages`, `/admin/schedules`.
- Admin UI gains Conversations and Schedules tabs.
- Admin **cannot** mutate user data via UI (no pin / no runtime switch / no delete) — operators must use SQL for that.

## Capabilities

### New Capabilities
- `chat-conversations`: conversation lifecycle including pinning, per-conversation runtime+model, cursor-based listing, deletion cascade, and turn source tagging.
- `chat-scheduler`: per-conversation scheduling — schema, CRUD APIs, dispatcher that fires prompts into the originating conversation, and isolation rules.
- `mcp-tools`: `./perch mcp` stdio sub-mode and the three scheduler tools, including identity-from-env contract.
- `admin-conversations-readonly`: admin read-only endpoints and UI surface for browsing conversations, messages, and schedules across all users.

### Modified Capabilities
- `multi-turn-chat`: drop the 24-hour history window and the 20-turn cap. Conversation history is loaded from the `conversations` + `query_sessions` tables for the conversation the user is in, with no time cutoff.
- `chat-api-acp`: `session/new` now passes a non-empty `mcpServers` list (the perch MCP sub-mode) and threads identity env vars to it.
- `agent-runtime-selection`: per-conversation runtime+model is now persisted on the conversation row and exposed via `GET /api/runtimes` for the picker.
- `chat-ui`: add the pin button, runtime/model dropdown, scheduler button, "Load More" affordance, and ⏰ source badge.
- `management-history` (no requirement change — kept as-is; new admin views land in `admin-conversations-readonly`).

## Impact

- **Schema migrations**: new columns on `conversations` (`pinned`, `pinned_at`, `runtime`, `model`) and `query_sessions` (`source`); new `chat_schedules` table + index. SQLite `ALTER TABLE` is non-blocking; existing rows get safe defaults.
- **Affected code**: `store_conversations.go`, `store.go`, `scheduler.go`, `chat_api_acp.go`, `acp_process.go`, `server.go`, `auth.go` / `gitlab_auth.go`; new files `mcp_server.go` (sub-mode) and `chat_schedules.go`.
- **API surface**: 4 new user endpoints + 4 new admin endpoints + 1 new `GET /api/runtimes`; one existing endpoint (`DELETE /api/conversations/{id}`) gets a cascade extension.
- **Binary surface**: `./perch mcp` sub-mode added to `main.go`; same binary, new mode flag.
- **Frontend**: chat page (header dropdown, pin, scheduler button, Load More), admin page (Conversations + Schedules tabs).
- **Backwards compatibility**: existing single-user installs migrate transparently. The 24-hour history removal is **BREAKING** for the multi-turn-chat spec contract — clients that depended on history being trimmed at 24h must adapt (in practice this is server-internal, no public API change).
- **Runtime compatibility risk**: `mcpServers` support varies by runtime; rollout is claude → codex → opencode → sst. The UI scheduler button works regardless of runtime.
