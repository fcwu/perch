## ADDED Requirements

### Requirement: ./perch mcp sub-mode runs an stdio MCP server

The perch binary SHALL accept an `mcp` sub-command that runs a stdio Model Context Protocol (MCP) server on the process's stdin / stdout. When invoked as `./perch mcp`, the process SHALL NOT start the HTTP server, scheduler ticker, or any other top-level subsystem; it SHALL only serve MCP requests until stdin closes.

#### Scenario: mcp sub-mode boots and responds to initialize

- **WHEN** `./perch mcp` is invoked with `PERCH_USER_ID`, `PERCH_CONV_ID`, `PERCH_DB_PATH` set in env, and an MCP `initialize` request is written to stdin
- **THEN** the process responds with a valid MCP initialize response advertising the three scheduler tools
- **AND** does NOT bind any TCP port

#### Scenario: mcp sub-mode exits cleanly on stdin EOF

- **WHEN** the parent runtime closes its stdin pipe to the MCP subprocess
- **THEN** the perch mcp process exits with code 0 within 1 second

### Requirement: Identity is locked from environment variables

The perch mcp process SHALL read its operating identity (`user_id`, `conversation_id`) and database path from `PERCH_USER_ID`, `PERCH_CONV_ID`, and `PERCH_DB_PATH` at startup. These values SHALL NOT be overridable by any MCP request, tool argument, or in-band agent input. If any of the three env vars is missing or empty, the process SHALL exit non-zero with a diagnostic message before serving any tool call.

#### Scenario: Missing env causes early exit

- **WHEN** `./perch mcp` is started without `PERCH_USER_ID` set
- **THEN** the process writes "perch mcp: PERCH_USER_ID is required" to stderr and exits with non-zero status
- **AND** does NOT serve any MCP tool call

#### Scenario: Tool argument cannot override identity

- **WHEN** the agent calls `schedule_message` with extra fields like `{"user_id":"someone-else", "conversation_id":"<other>", ...}`
- **THEN** the MCP server SHALL ignore those fields and use the values bound from env at startup
- **AND** the tool description SHALL NOT advertise `user_id` or `conversation_id` as parameters

#### Scenario: DB writes are scoped to env-bound identifiers

- **WHEN** any of the three tools writes or reads `chat_schedules`
- **THEN** the SQL `WHERE` / `INSERT` SHALL use the env-bound `PERCH_USER_ID` and `PERCH_CONV_ID`

### Requirement: schedule_message tool

The MCP server SHALL expose a tool named `schedule_message` accepting `{"prompt": string, "hour"?: integer 0-23, "minute"?: integer 0-59, "repeat"?: boolean, "one_shot_at"?: integer epoch-ms}`. Exactly one of (`hour`+`minute`) OR `one_shot_at` SHALL be provided. The tool SHALL validate the inputs, insert a new `chat_schedules` row scoped to the env-bound identity, and return `{"id": "<job_id>"}`.

#### Scenario: Daily schedule via tool

- **WHEN** the agent calls `schedule_message({"prompt":"summarise yesterday","hour":9,"minute":0,"repeat":true})`
- **THEN** a row is inserted with `user_id=PERCH_USER_ID`, `conversation_id=PERCH_CONV_ID`, `hour=9, minute=0, repeat=1, one_shot_at=0, prompt="summarise yesterday"`
- **AND** the response is `{"id":"<job-id>"}`

#### Scenario: One-shot schedule via tool

- **WHEN** the agent calls `schedule_message({"prompt":"check the build","one_shot_at":<future-ms>})`
- **THEN** a row is inserted with `one_shot_at=<future-ms>, repeat=0`

#### Scenario: Invalid input rejected

- **WHEN** the agent calls `schedule_message` with neither `hour+minute` nor `one_shot_at`, or with `hour=25`, or with `one_shot_at` in the past
- **THEN** the tool returns an error with a human-readable `message` field and inserts no row

### Requirement: list_schedules tool

The MCP server SHALL expose a tool `list_schedules` taking no arguments. It SHALL return an array of all `chat_schedules` rows scoped to the env-bound identity where `enabled=1`, each row including `id, hour, minute, repeat, one_shot_at, prompt, created_at, last_fired_at`.

#### Scenario: Returns only the env-bound conversation's schedules

- **WHEN** the agent calls `list_schedules()`
- **THEN** the response includes rows for `user_id=PERCH_USER_ID AND conversation_id=PERCH_CONV_ID AND enabled=1` only
- **AND** disabled rows and rows belonging to other conversations are NOT included

#### Scenario: Empty result is an empty array

- **WHEN** the user has no schedules in this conversation
- **THEN** the tool returns `{"schedules": []}`

### Requirement: cancel_schedule tool

The MCP server SHALL expose a tool `cancel_schedule({"id": string})`. It SHALL delete the row only when both `user_id` and `conversation_id` match the env-bound identity. The tool SHALL return `{"deleted": true}` on success, `{"deleted": false}` if no matching row exists.

#### Scenario: Cancel own schedule

- **WHEN** the agent calls `cancel_schedule({"id":"<own-id>"})`
- **THEN** the row is deleted and the response is `{"deleted":true}`

#### Scenario: Cancel non-existent or foreign schedule

- **WHEN** the agent calls `cancel_schedule({"id":"<unknown-or-foreign-id>"})`
- **THEN** no row is deleted and the response is `{"deleted":false}` (no error)

### Requirement: MCP server enforces a per-tool deadline

Each MCP tool invocation SHALL time out after 5 seconds; if the underlying SQLite query has not completed, the server SHALL return an error to the runtime so the agent receives a definitive result rather than hanging.

#### Scenario: Slow DB does not hang the runtime

- **WHEN** a tool call takes longer than 5 seconds (e.g., DB lock contention)
- **THEN** the server returns `{"error":"timeout"}` and the agent's call site receives a normal error response
