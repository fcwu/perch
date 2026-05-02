## ADDED Requirements

### Requirement: AgentRuntime carries SupportsMCP and a model list

The `AgentRuntime` struct SHALL declare `SupportsMCP bool` and `Models []string` and `DefaultModel string` fields. `loadAgentRuntime` SHALL populate them per the configured runtime so that the chat-API can decide whether to pass `mcpServers`, and `GET /api/runtimes` can advertise a model list to the picker.

#### Scenario: claude runtime advertises MCP support and a Claude model list

- **WHEN** `AGENT_RUNTIME=claude`
- **THEN** the returned runtime has `SupportsMCP=true`, `Models` includes at least one `claude-*` id, and `DefaultModel` is set to the registry's recommended default

#### Scenario: codex / opencode / sst runtimes default to SupportsMCP=false until verified

- **WHEN** `AGENT_RUNTIME=codex` or `opencode` or `sst`
- **THEN** the returned runtime SHALL have `SupportsMCP=false` until the runtime is smoke-tested and the flag is flipped in the registry

### Requirement: Conversation-level runtime selection at POST /api/chat boundary

When `POST /api/chat` arrives with `(user_id, conversation_id)`, the server SHALL look up the conversation's `runtime` and `model` fields and use those values — not a server-wide default — to determine which `AgentRuntime` to use for the ACP session. If the conversation row's `runtime` is NULL (legacy data), the server SHALL backfill it with the current default and persist the row.

#### Scenario: Conversation runtime overrides server default

- **WHEN** the server default is `claude` but the conversation row has `runtime=opencode`
- **THEN** the ACP subprocess for this conversation is `opencode acp ...`, not `claude-agent-acp`

#### Scenario: NULL runtime is lazily backfilled

- **WHEN** a legacy conversation row has `runtime=NULL` and the user submits a message
- **THEN** the server sets the row's `runtime` and `model` to the current defaults via UPDATE, then proceeds with the ACP session

### Requirement: GET /api/runtimes advertises available runtimes and models

The endpoint `GET /api/runtimes` SHALL return the list of runtimes the server can spawn, with each entry containing `id`, `name`, `models[]`, `default_model`, `supports_mcp`. The list SHALL be derived from the runtime registry; the response SHALL be authentication-gated per the multi-user mode (single-user mode permits unauthenticated calls).

#### Scenario: Authenticated request returns runtime list

- **WHEN** an authenticated user calls `GET /api/runtimes`
- **THEN** the response is `{"runtimes":[{"id":"claude","name":"Claude","models":[...],"default_model":"...","supports_mcp":true}, ...]}`

#### Scenario: Unauthenticated request in multi-user mode is rejected

- **WHEN** the server runs in multi-user mode and a request without a valid session hits `GET /api/runtimes`
- **THEN** the server returns 401 (consistent with other `/api/*` endpoints)
