## ADDED Requirements

### Requirement: ACP run lifecycle events drive ManagementHub session state

When an ACP prompt for a chat-API session is started, progresses, or completes, perch SHALL drive `ManagementHub` events directly from the ACP event stream, not from the legacy `/hook` endpoint.

> Note: this Requirement applies to chat-API ACP sessions only. Discord and Telegram ACP sessions do NOT feed into ManagementHub by default (they're IM-driven, not query-tracked). See Open Question Q3 in design.md.

#### Scenario: Prompt start emits SessionAdded

- **WHEN** chat-API issues an ACP `prompt(sessionID, queryText)` against a pooled session
- **THEN** perch calls `ManagementHub.SessionAdded(ManagementSessionView{ID: sessionID, Username: <user>, Query: <queryText>, Status: "running", StartedAt: <now>})` immediately after dispatching the prompt

#### Scenario: tool_call_started emits SessionUpdated

- **WHEN** ACP emits `tool_call_started{tool_name: "Bash", ...}` for an active prompt
- **THEN** perch calls `ManagementHub.SessionUpdated(sessionID, "Bash")`

#### Scenario: tool_call_completed clears current_tool

- **WHEN** ACP emits `tool_call_completed` for the currently running tool
- **THEN** perch calls `ManagementHub.SessionUpdated(sessionID, "")` to clear the `current_tool` field

#### Scenario: RunCompleted emits SessionRemoved with status=done

- **WHEN** ACP emits `RunCompleted` for the active prompt
- **THEN** perch calls `ManagementHub.SessionRemoved(sessionID, "done")`

#### Scenario: RunFailed emits SessionRemoved with status=error

- **WHEN** ACP emits `RunFailed` (or a timeout occurs)
- **THEN** perch calls `ManagementHub.SessionRemoved(sessionID, "error")`

### Requirement: ACP run lifecycle events drive query log store writes

The chat-API path SHALL persist queries, responses, and tool events into the sqlite `query_sessions` and `tool_events` tables based on ACP events, not hook events.

#### Scenario: Prompt start inserts query_sessions row

- **WHEN** chat-API issues an ACP prompt
- **THEN** perch inserts a row into `query_sessions` with `id`, `user_id`, `username`, `query`, `started_at`, `status="running"`, `conversation_id`

#### Scenario: tool_call_started inserts tool_events row

- **WHEN** ACP emits `tool_call_started{tool_name, input}`
- **THEN** perch inserts a row into `tool_events` with `session_id`, `tool_name`, `input_json`, `started_at`

#### Scenario: tool_call_completed updates the matching tool_events row

- **WHEN** ACP emits `tool_call_completed{output}` matching a previously inserted `tool_events` row
- **THEN** perch updates that row with `output_json` and `ended_at`

#### Scenario: RunCompleted finalizes query_sessions row

- **WHEN** ACP emits `RunCompleted` with the accumulated assistant response
- **THEN** perch updates the matching `query_sessions` row with `response`, `ended_at`, `status="done"`

#### Scenario: RunFailed finalizes query_sessions row with error status

- **WHEN** ACP emits `RunFailed` or a timeout occurs
- **THEN** perch updates the matching `query_sessions` row with `response="<error message>"`, `ended_at`, `status="error"`

### Requirement: Hook system is removed and not the source of management observability

After this change, perch SHALL NOT have a `/hook` HTTP endpoint, SHALL NOT depend on `claude/settings.json` hook injection, and SHALL NOT route any `HookEvent` to ManagementHub or query_log_store.

#### Scenario: /hook endpoint is gone

- **WHEN** any client (including legacy claude-installed hooks) sends `POST /hook` to perch
- **THEN** the request returns 404 Not Found
- **AND** no internal handler is invoked

#### Scenario: claude/settings.json no longer injected

- **WHEN** the perch container starts
- **THEN** `entrypoint.sh` does not call `merge-settings.js`
- **AND** `$WORKDIR/.claude/settings.json` is untouched by perch (any hooks present are user-defined and outside perch's concern)

#### Scenario: HookEvent type does not exist

- **WHEN** the perch source is grepped for `HookEvent`
- **THEN** no Go file references the type
- **AND** the `IMAdapter` interface does not declare `Notify(HookEvent, string) error`
