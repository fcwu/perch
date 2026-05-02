## Context

Perch ships a web chat backed by ACP (Agent Client Protocol) subprocesses, plus Discord and Telegram adapters. Today:

- `conversations` table exists (`store_conversations.go:22-28`) but only stores `id, user_id, title, created_at, updated_at`. No pinning, no runtime/model, listing is `ORDER BY updated_at DESC LIMIT 50`.
- ACP per-(user, conv) sessions live in a pool with a 15-minute idle timeout (`acp_session_pool.go:14`); pool eviction triggers an `evictHook` that deletes `<workdir>/uploads/<conv-id>/`.
- Conversation history injection follows the `multi-turn-chat` spec — a 24-hour sliding window over `query_sessions`, capped at 20 turns. (Note: with ACP per-conversation persistent sessions in `chat-api-acp`, the 24h injection has been bypassed in the ACP path; we are formalizing that.)
- `scheduler.go` already exists for daily-fire jobs (Hour/Minute + Repeat) targeted at the main PTY or `discord:<channelID>`. It dispatches via a hook function. There is no notion of `user_id` on a job today and no `chat:` target.
- ACP `session/new` is called with `mcpServers: []any{}` (`acp_process.go:227-230`) — the insertion point for our self-hosted MCP server.
- Multi-user mode is gated by GitLab OAuth; `admin` role is recognized, and existing admin endpoints (`/admin/history`, `/admin/analytics`) read aggregated `query_sessions` rows.
- Conversation deletion endpoint already exists (`server.go:751`) — only the cascade for the new `chat_schedules` table needs to be added.

Constraints:

- SQLite (single-file, embedded) — schema migration is `ALTER TABLE ADD COLUMN` style; safe additive changes only.
- Process model: a single perch binary acts as web server, scheduler ticker, ACP supervisor, Discord/Telegram bots. We will reuse the binary for the MCP sub-mode.
- Multi-runtime: claude, codex, opencode, sst — different ACP transports, different MCP-support maturity.
- Single-user mode (no auth) must continue working transparently.

## Goals / Non-Goals

**Goals:**

1. Long-lived conversations: indefinite retention, pinning, mid-conversation runtime+model switching.
2. Per-conversation scheduler with two equivalent entry points (UI button, agent tool) that fire prompts back into the originating conversation only.
3. Strict multi-user isolation: a user cannot see, modify, or schedule against another user's conversation. Admins get read-only visibility into all of it.
4. Reuse existing infrastructure (ACP pool, scheduler ticker, conversations table) — no parallel data plane.

**Non-Goals:**

- Cron expressions on schedules. v1 is daily H/M+repeat OR one-shot epoch-ms. Cron, weekly, monthly come later.
- Cross-conversation tooling (e.g., "summarize my last 5 conversations"). Out of scope; tools are conv-scoped.
- Custom agent profiles (system prompt + tool allowlist per agent). Out of scope; runtime+model is the only knob in v1.
- Admin write actions on user data via UI. Operators must use SQL for that — keeps the blast radius of admin UI bugs small.
- Telegram and Discord adapters' parity for the new scheduler. They keep using the existing PTY/discord scheduler. Web chat is the focus.

## Decisions

### D1. Reuse `conversations` table; ALTER ADD COLUMN

**Decision**: add `pinned INTEGER DEFAULT 0`, `pinned_at INTEGER`, `runtime TEXT`, `model TEXT` directly to the existing `conversations` table.

**Alternatives**:
- Side table `conversation_meta` keyed by id — rejected because every list/list-after-cursor query already touches `conversations`; a join per row adds cost without isolation benefit.
- JSON blob `metadata` column — rejected for the runtime+model fields because we want to filter by them later (e.g., admin "show me all opencode threads").

### D2. Cursor pagination keyed by `updated_at`, pinned-as-prefix

**Decision**: `GET /api/conversations?before=<updated_at_ms>&limit=20`. Pinned rows are returned at the top of every page (until the page contains a non-pinned row whose `updated_at < before`). Concretely the SQL is two queries:

```sql
-- Pinned rows (only on the first page; client passes ?include_pinned=1 implicitly)
SELECT ... FROM conversations
WHERE user_id=? AND pinned=1
ORDER BY pinned_at DESC;

-- Recent rows (cursor)
SELECT ... FROM conversations
WHERE user_id=? AND pinned=0 AND (? = 0 OR updated_at < ?)
ORDER BY updated_at DESC LIMIT ?;
```

**Why**: `OFFSET` pagination is unstable when rows get touched (a user typing in conversation X mid-scroll shifts indices); cursor on a monotonic field is the standard fix. Returning pinned rows separately on the first page only avoids re-emitting them on subsequent "Load More" calls.

**Alternatives**:
- Single query with `ORDER BY pinned DESC, updated_at DESC` — cursor becomes a tuple `(pinned, updated_at)`; client logic gets messy. Two-query form is clearer.
- Keep `LIMIT 50` and just remove the 24h cap — rejected: 50 is hit fast for active users.

### D3. Mid-conversation runtime switch starts a new ACP session

**Decision**: when `PATCH /api/conversations/{id}` changes `runtime` or `model`, the corresponding pool key (`chat-api:<user>:<conv>`) is **evicted immediately** so that the next prompt boots a fresh ACP subprocess with the new runtime. The UI `query_sessions` history is unchanged; the new ACP session has no in-process memory of prior turns. The user is told this in the dropdown UI ("Switching agents starts a fresh context").

**Alternatives considered**:
- Replay prior turns into the new ACP session before the next user prompt — rejected for v1: each runtime has a different prompt-window cost model, and replay can fail mid-way; the UX of "agent starts fresh" is simpler and predictable. Can revisit later.
- Disallow switching mid-conversation — rejected; the user explicitly asked for it.

### D4. Scheduler dispatch reuses `scheduler.go` ticker, adds `chat:<convID>` target

**Decision**: keep the existing `Scheduler` 30-second ticker and `dispatch` hook. Add a new dispatch branch for `chat:<convID>` targets that:
1. Looks up the conversation row to obtain `user_id`, `runtime`, `model`.
2. Submits the prompt as a synthetic user turn through the same code path that `POST /api/chat` uses (insert `query_sessions` with `source='schedule'`, run the prompt through the conversation's ACP session, stream the reply, mark done).
3. Fans the SSE chunks out to any web client currently subscribed to that conversation.

**Why**: a schedule firing produces the same turn structure as a manual user message; the only differences are who triggered it and the `source` tag. Keeping it on the same dispatch path means `tool_call_stream`, `acp-tool-events`, and persistence all work for free.

**Schedule storage**: the `chat_schedules` table is canonical. We do **not** persist these rows into `.perch/schedules.jsonl` (which remains the home for the legacy daily PTY/Discord jobs). On startup, scheduler loads the JSONL **and** queries `chat_schedules` to populate its in-memory job map.

**Alternatives**:
- Build a separate goroutine for chat schedules — rejected: duplicates ticker logic, and the existing `fireDue` is small enough to extend.
- Store everything in JSONL — rejected: we need to filter by user_id and conversation_id from web APIs, which is awkward against a flat file.

### D5. MCP sub-mode, identity locked from env

**Decision**: add `./perch mcp` sub-command to the existing binary. It runs an stdio MCP server that exposes three tools:

```
schedule_message(time, prompt, repeat?, one_shot_at?)
list_schedules()
cancel_schedule(id)
```

It reads `PERCH_USER_ID`, `PERCH_CONV_ID`, `PERCH_DB_PATH` from env at startup, opens the SQLite DB read-write, and performs all operations scoped to those identifiers. The agent is **never** asked for user_id or conversation_id — those values are not in the tool schema at all.

**Why identity-from-env**: the MCP server is invoked as a subprocess by the runtime, which is in turn invoked as a subprocess by perch. Perch sets the env when launching the runtime; the env is inherited. The agent (running inside the runtime) cannot mutate the MCP subprocess's env. This is the only safe way to bind identity given that agent input is fundamentally untrusted (prompt injection from user input or tool output can try to coerce the agent into "user_id=admin").

**Why same binary**: avoids shipping a second binary; reduces Docker image surface; the MCP code is small (a few hundred lines).

**Alternatives**:
- HTTP MCP over a Unix socket with per-session bearer tokens — rejected: more moving parts (token issuance, socket cleanup), no safety advantage over env (both rely on perch's process boundary).
- A separate `perch-mcp` binary — rejected: extra build/release complexity, and the binary would need to import most of perch's data layer anyway.
- Don't use MCP, parse magic markers in agent output — rejected: brittle, race-prone, and the user explicitly asked for a real tool.

### D6. Runtime compatibility: claude first, others verified one-by-one

**Decision**: `chat_api_acp.go` always sends a non-empty `mcpServers` list. Runtimes that don't support `mcpServers` may either ignore it (fine) or error during `session/new` (we'd see it as a startup failure). For v1:

- **claude-agent-acp**: known to support `mcpServers`. Primary target.
- **codex / opencode / sst**: smoke-test during implementation. If a runtime errors on `mcpServers`, we either: (a) detect and pass empty for that runtime via an `AgentRuntime.SupportsMCP` flag we add, or (b) keep sending and let it fail loudly (unacceptable). We will go with (a) — the flag defaults to `false` for unverified runtimes, gets flipped to `true` once tested. UI button still works for everyone.

**Trade-off**: users on opencode/codex initially won't be able to ask the agent "schedule X for tomorrow" — they'll have to use the UI. Acceptable for v1.

### D7. Scheduler-fired turns are tagged `source='schedule'` on `query_sessions`

**Decision**: add `source TEXT DEFAULT 'user'` to `query_sessions`. Manual chat messages keep the default `'user'`. The scheduler-dispatch path inserts with `source='schedule'`. The `GET /api/conversations/{id}/messages` (and admin equivalent) include `source`. The chat UI renders a small ⏰ badge on `source='schedule'` rows.

**Why on `query_sessions` and not a join table**: turns are query_sessions; the source is a property of the turn itself. A side table would require a join on every render.

### D8. Admin views are read-only by design

**Decision**: admin endpoints serve `SELECT`-only — no PATCH, no DELETE. To delete or pin on behalf of a user, an operator opens `sqlite3` against the DB.

**Why**: the cost of an admin UI bug is high — a misclick deletes user data. Read-only admin gives observability without a destructive surface. Mutations from SQL leave a clear audit trail in shell history and require explicit operator intent.

**Alternative considered**: admin can mutate but with a confirmation dialog — rejected for v1; the bar to add later is low if we find we need it.

### D9. Single-user mode unchanged

`resolveUserID()` already returns `"default"` in single-user mode. All new SQL filters use the same value, so single-user installs migrate without any code path branching.

## Risks / Trade-offs

- **Risk**: Long retention grows the SQLite file unboundedly.
  → **Mitigation**: not blocking v1. We'll add a "Bulk Delete" affordance in a follow-up, plus a settings knob like `CHAT_RETENTION_DAYS` that the user can opt into. SQLite handles tens of millions of rows fine.

- **Risk**: An MCP subprocess crash / hang during agent tool call could stall the runtime.
  → **Mitigation**: the MCP server sets a 5-second deadline on each tool RPC; on deadline exceeded it returns an error to the runtime. Subprocess respawn is handled by the runtime's MCP client.

- **Risk**: Runtime that ignores `mcpServers` silently still works, but agent thinks tools exist that it can't actually call.
  → **Mitigation**: only send `mcpServers` when `AgentRuntime.SupportsMCP` is true; else send empty. The agent never sees the tool description.

- **Risk**: Cursor pagination + frequent `TouchConversation` calls cause a row to skip over the cursor between page loads.
  → **Mitigation**: pinned rows are always returned on the first page only. For non-pinned rows, the cursor uses `updated_at < before`; if a row gets touched mid-scroll its new `updated_at` is greater than the original cursor and won't appear in subsequent pages — acceptable, the user already saw it.

- **Risk**: Schedule storm (e.g., 100 schedules at 09:00 in different conversations) ties up runtime concurrency.
  → **Mitigation**: existing ACP pool global LRU eviction kicks in; schedules naturally serialize per-(user, conv). Cross-conversation parallelism is bounded by the pool size. No new throttling needed in v1.

- **Risk**: Schedule fires for a conversation whose configured runtime has been removed or model has been retired.
  → **Mitigation**: `ChatSchedule.Fire()` looks up the conversation row, validates `runtime/model` against the runtime registry, and on mismatch logs + skips (does not delete the schedule, since the operator may restore the runtime).

- **Risk**: Admin sees user data → privacy concern.
  → **Mitigation**: admin role is gated by `ADMIN_TOKEN` env (existing). Read-only is the second line of defense. Document explicitly in operator docs that admin UI surfaces all conversations.

## Migration Plan

1. **Schema** (one DB session, idempotent):
   ```sql
   ALTER TABLE conversations ADD COLUMN pinned INTEGER DEFAULT 0;
   ALTER TABLE conversations ADD COLUMN pinned_at INTEGER;
   ALTER TABLE conversations ADD COLUMN runtime TEXT;
   ALTER TABLE conversations ADD COLUMN model TEXT;
   ALTER TABLE query_sessions ADD COLUMN source TEXT DEFAULT 'user';
   CREATE TABLE IF NOT EXISTS chat_schedules ( /* per spec */ );
   CREATE INDEX IF NOT EXISTS idx_chat_schedules_user_conv ON chat_schedules(user_id, conversation_id);
   ```
   Run on perch startup via a versioned migration step.

2. **Backfill `runtime` + `model`**: existing conversations get the server's current default runtime+model on first read (lazy backfill: if the row's `runtime` is NULL, populate it with `effectiveRuntime()` and `UPDATE`).

3. **Deploy order**:
   - DB migration & backend (single deploy; schema changes are additive).
   - Frontend bundle (chat header + admin tabs).
   - MCP sub-mode + ACP `session/new` change (gated by `AgentRuntime.SupportsMCP=true` for claude only initially).

4. **Rollback**: backend rollback to the prior binary is safe — the new columns sit unused, and the new `chat_schedules` table is queried only by code paths that have been removed. No destructive schema changes to undo.

## Open Questions

- **Q**: Should one-shot schedules be deleted from `chat_schedules` after firing, or marked `enabled=0` for audit? → Default: delete on fire; can revisit if operators ask for an audit trail.
- **Q**: Should the agent tool `list_schedules()` return only enabled schedules, or include disabled ones? → v1: only enabled (matches what the user can act on).
- **Q**: How does the chat UI surface "this runtime doesn't support agent tools"? → Probably a small footnote in the runtime dropdown ("Codex: agent cannot self-schedule yet"). Detail is UI work; not a backend concern.
- **Q**: Discord/Telegram adapters — do they ever need the new scheduler? Not in v1; their existing `discord:<channelID>` PTY scheduler stays as-is. If we want parity later, we can extend `dispatch` similarly.
