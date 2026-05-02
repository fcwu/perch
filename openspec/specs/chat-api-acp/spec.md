## ADDED Requirements

### Requirement: Chat-API runs queries through ACP per-conversation persistent sessions

When the user submits a query via `/api/chat`, perch SHALL acquire (or create) a per-(user, conversation) ACP subprocess from the session pool, submit the query as an ACP `prompt`, stream `agent_message_chunk` and tool events back to the browser, and keep the subprocess alive for subsequent prompts in the same conversation until idle timeout. The runtime executable used for a given conversation SHALL be selected from the conversation's `runtime` column (and the `model` column where applicable), not a server-global flag.

#### Scenario: First query in a conversation creates a new ACP session for the conversation's runtime

- **WHEN** `POST /api/chat` arrives with `(user_id, conversation_id)` not yet in the pool
- **THEN** perch reads the conversation's `runtime` and `model` fields and starts a subprocess for that runtime (e.g., `claude-agent-acp` for `runtime=claude`, `opencode acp --log-level WARN` for `runtime=opencode`)
- **AND** runs ACP `initialize` followed by `new_session` with `permissionMode: "bypassPermissions"`, `workspace_path` set to the perch workspace, and `mcpServers` populated per the runtime's `SupportsMCP` flag
- **AND** stores the resulting ACP session ID in the pool under key `chat-api:<user_id>:<conversation_id>`
- **AND** issues `prompt(sessionID, queryText)` against the new session

#### Scenario: Subsequent query in the same conversation reuses the session

- **WHEN** `POST /api/chat` arrives with the same `(user_id, conversation_id)` and the pooled subprocess is still alive
- **THEN** perch issues `prompt(sessionID, queryText)` directly without re-initializing
- **AND** the runtime retains conversation context from the previous prompts (no need to re-prepend history)

#### Scenario: Idle timeout reclaims subprocess

- **WHEN** no `prompt` is issued against a pooled session for the configured idle window (default 15 minutes)
- **THEN** perch terminates the ACP subprocess and removes the entry from the pool
- **AND** a subsequent `POST /api/chat` for the same `(user_id, conversation_id)` triggers a fresh subprocess start

#### Scenario: Subprocess crashes mid-conversation

- **WHEN** the ACP subprocess for a pooled session exits unexpectedly between prompts
- **THEN** the next `POST /api/chat` for that key starts a new subprocess and a new ACP session
- **AND** logs the crash with the previous session's exit status

#### Scenario: Per-user pool limit enforced via LRU

- **WHEN** a user already has the per-user limit (default 5) of pooled subprocesses and submits a query for a new conversation
- **THEN** the least-recently-used pooled subprocess for that user is terminated to make room
- **AND** the new subprocess starts for the requested conversation

#### Scenario: Runtime/model change forces session restart

- **WHEN** the conversation's `runtime` or `model` field changes (via `PATCH /api/conversations/{id}`)
- **THEN** the pool entry for `chat-api:<user>:<conv>` is evicted immediately
- **AND** the next `POST /api/chat` boots a fresh subprocess for the new runtime/model

### Requirement: Chat-API responses stream from ACP, not PTY

The browser `EventSource` connection at `/api/chat/stream` (or WebSocket at `/ws/chat`) SHALL receive its data from ACP `agent_message_chunk`, `tool_call_started`, and `tool_call_completed` events translated into the existing client-facing JSON event format, NOT from PTY stdout drain.

#### Scenario: Text response is streamed chunk by chunk

- **WHEN** the ACP subprocess emits `agent_message_chunk` events containing assistant text
- **THEN** perch translates each chunk into the existing `{type: "text", data: "..."}` SSE/WS event format
- **AND** sends each event to the connected browser without buffering for whole-response

#### Scenario: Tool events are forwarded as structured JSON

- **WHEN** the ACP subprocess emits `tool_call_started` or `tool_call_completed`
- **THEN** perch translates these into the existing tool-event JSON format
  - `tool_call_started` → `{type: "tool_use", tool_name: "...", input: {...}}`
  - `tool_call_completed` → `{type: "tool_result", tool_name: "...", output: {...}}`
- **AND** sends them to the connected browser

#### Scenario: Stream closes on RunCompleted

- **WHEN** ACP emits `RunCompleted` for the active prompt
- **THEN** perch sends a `{type: "done"}` event to the browser
- **AND** closes the SSE/WS connection cleanly

#### Scenario: Stream closes on RunFailed

- **WHEN** ACP emits `RunFailed` (or a timeout occurs)
- **THEN** perch sends a `{type: "error", message: "..."}` event to the browser
- **AND** closes the SSE/WS connection

### Requirement: Chat-API does NOT spawn `claude -p` subprocesses

After this change, the chat-API path SHALL NOT invoke `claude -p` (or any non-ACP Claude Code CLI mode) for query handling. Claude is reached exclusively via the ACP subprocess.

#### Scenario: No `claude -p` process appears under chat-API load

- **WHEN** `/api/chat` receives a sequence of queries
- **THEN** `pgrep -af "claude -p"` (executed inside the container) returns no matching process at any point
- **AND** all responses are served from `claude-agent-acp` subprocesses

#### Scenario: Image still contains `claude` binary for web `/ws`

- **WHEN** the perch image is built
- **THEN** the `claude` binary is still installed (used by web `/ws` interactive terminal)
- **AND** the chat-API code path does not invoke it directly

### Requirement: Chat-API accepts image attachments alongside the text query

`POST /api/chat` SHALL accept an optional `attachments` array of `{filename, mime_type, data_base64}` objects. The server SHALL forward validated attachments to ACP as `image` content blocks in the same `session/prompt` call as the text query.

#### Scenario: Pure-text query is unchanged

- **WHEN** the client posts `{"query":"hi","new_conversation":true}` (no `attachments` field)
- **THEN** the server SHALL behave exactly as before — single text content block, no validation overhead

#### Scenario: Query with one image is forwarded to ACP

- **WHEN** the client posts `{"query":"what's wrong here?","attachments":[{"filename":"err.png","mime_type":"image/png","data_base64":"<b64>"}]}`
- **THEN** the server SHALL call `PromptWithContent(ctx, [{type:"text",text:"what's wrong here?"}, {type:"image",data:"<b64>",mimeType:"image/png"}], ...)` (flat ACP `ImageContent` schema, NOT Anthropic-style nested `source`)

#### Scenario: Server validates MIME, size, and count

- **WHEN** the server receives `attachments`
- **THEN** the server SHALL reject (HTTP 400) when:
  - any attachment's `mime_type` is not in `CHAT_UPLOAD_ALLOWED_MIME` (default: `image/png,image/jpeg,image/gif,image/webp`)
  - any attachment's decoded byte size exceeds `CHAT_UPLOAD_MAX_BYTES` (default: 10 MB)
  - the magic bytes of decoded data do not match the claimed `mime_type`
  - `len(attachments) > CHAT_UPLOAD_MAX_FILES` (default: 4)

#### Scenario: query field shows attachment placeholder in management history

- **WHEN** a chat-API query with attachments completes
- **THEN** `query_sessions.query` SHALL contain the placeholder-prefixed form `[image:<filename1>] [image:<filename2>] <original text>` so the management history list does not embed base64 data

#### Scenario: Attachments are not persisted

- **WHEN** the ACP run completes
- **THEN** the server SHALL NOT write attachment bytes to `/data`, the workspace, or `query_log_store`; the attachment bytes live only in process memory for the duration of the prompt

### Requirement: ACP session/new passes mcpServers when runtime supports MCP

When the runtime's `SupportsMCP` flag is true, perch SHALL include in the ACP `session/new` request an `mcpServers` array describing the perch self-hosted MCP server (the `./perch mcp` sub-mode), with the env entries `PERCH_USER_ID`, `PERCH_CONV_ID`, and `PERCH_DB_PATH` populated. When `SupportsMCP` is false, perch SHALL pass `mcpServers: []`.

#### Scenario: Claude runtime gets the perch MCP server

- **WHEN** `(user, conv)` runs against `runtime=claude` and `SupportsMCP=true`
- **THEN** the `session/new` request body includes `mcpServers: [{ "type":"stdio", "command":"<path-to-perch>", "args":["mcp"], "env": { "PERCH_USER_ID":"<user>", "PERCH_CONV_ID":"<conv>", "PERCH_DB_PATH":"<absolute-db-path>" } }]`

#### Scenario: Unverified runtime gets empty mcpServers

- **WHEN** `(user, conv)` runs against a runtime whose `SupportsMCP=false`
- **THEN** the `session/new` request body includes `mcpServers: []`
- **AND** the agent does not see the schedule_message / list_schedules / cancel_schedule tools

#### Scenario: Identity env never sourced from request input

- **WHEN** the request body to `POST /api/chat` includes a hypothetical `user_id` override field
- **THEN** the `mcpServers[*].env.PERCH_USER_ID` SHALL be set from the authenticated session's resolved user id, NOT from the request body
