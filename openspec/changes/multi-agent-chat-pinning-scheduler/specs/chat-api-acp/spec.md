## MODIFIED Requirements

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

## ADDED Requirements

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
