## ADDED Requirements

### Requirement: Chat-API runs queries through ACP per-conversation persistent sessions

When the user submits a query via `/api/chat`, perch SHALL acquire (or create) a per-(user, conversation) ACP subprocess from the session pool, submit the query as an ACP `prompt`, stream `agent_message_chunk` and tool events back to the browser, and keep the subprocess alive for subsequent prompts in the same conversation until idle timeout.

#### Scenario: First query in a conversation creates a new ACP session

- **WHEN** `POST /api/chat` arrives with `(user_id, conversation_id)` not yet in the pool
- **THEN** perch starts a new `claude-agent-acp` subprocess and runs ACP `initialize` followed by `new_session` with `permissionMode: "bypassPermissions"` and `workspace_path` set to the perch workspace
- **AND** stores the resulting ACP session ID in the pool under key `chat-api:<user_id>:<conversation_id>`
- **AND** issues `prompt(sessionID, queryText)` against the new session

#### Scenario: Subsequent query in the same conversation reuses the session

- **WHEN** `POST /api/chat` arrives with the same `(user_id, conversation_id)` and the pooled subprocess is still alive
- **THEN** perch issues `prompt(sessionID, queryText)` directly without re-initializing
- **AND** Claude retains conversation context from the previous prompts (no need to re-prepend history)

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
