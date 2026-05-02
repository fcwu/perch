# chat-scheduler Specification

## Purpose

Per-conversation scheduled prompts, including their storage in `chat_schedules`, CRUD endpoints scoped to (user, conversation), dispatcher integration with the existing `Scheduler.fireDue` ticker, and per-(user, conv) isolation.

## ADDED Requirements

### Requirement: chat_schedules table stores per-conversation scheduled prompts

The system SHALL persist scheduled prompts in a SQLite table `chat_schedules` with columns: `id TEXT PRIMARY KEY`, `user_id TEXT NOT NULL`, `conversation_id TEXT NOT NULL`, `hour INTEGER`, `minute INTEGER`, `repeat INTEGER` (0/1), `one_shot_at INTEGER` (epoch ms; 0 if not one-shot), `prompt TEXT NOT NULL`, `enabled INTEGER DEFAULT 1`, `created_at INTEGER`, `last_fired_at INTEGER`, with a foreign key on `conversation_id` referencing `conversations(id)` and an index on `(user_id, conversation_id)`.

A row SHALL represent **either** a daily-fire job (`hour` and `minute` set; `one_shot_at=0`; `repeat` controls auto-rearm) **or** a one-shot job (`one_shot_at` set; `hour=NULL`, `minute=NULL`, `repeat=0`).

#### Scenario: Daily schedule shape

- **WHEN** a daily 09:30 repeating schedule is created
- **THEN** the inserted row has `hour=9, minute=30, repeat=1, one_shot_at=0`

#### Scenario: One-shot schedule shape

- **WHEN** a one-shot schedule for 2026-05-03 09:00 UTC is created
- **THEN** the inserted row has `hour=NULL, minute=NULL, repeat=0, one_shot_at=<epoch ms of that moment>`

### Requirement: Schedule CRUD scoped to (user, conversation)

`GET /api/conversations/{id}/schedules` SHALL list schedules where `user_id` and `conversation_id` match the authenticated user and the path id, respectively. `POST /api/conversations/{id}/schedules` SHALL create a row with the same scoping; the body accepts `{prompt, hour?, minute?, repeat?, one_shot_at?}` with validation that exactly one of (hour+minute) or (one_shot_at) is provided. `DELETE /api/conversations/{id}/schedules/{job_id}` SHALL remove a row only if both `user_id` and `conversation_id` match.

#### Scenario: Owner lists own schedules

- **WHEN** the owner sends `GET /api/conversations/<id>/schedules`
- **THEN** the response is a JSON array of the user's schedules for that conversation, ordered by `created_at ASC`

#### Scenario: Non-owner gets empty list

- **WHEN** user A sends `GET /api/conversations/<B-conv-id>/schedules`
- **THEN** the response is 404 (treated as not-found)

#### Scenario: Cancel rejects another user's schedule

- **WHEN** user A sends `DELETE /api/conversations/<A-conv-id>/schedules/<B-job-id>`
- **THEN** the response is 404 and the row is unchanged

### Requirement: Scheduler dispatch fires prompts into the originating conversation

When the perch scheduler ticker matches a `chat_schedules` row, the dispatcher SHALL: (1) load the conversation row to obtain `runtime` and `model`; (2) insert a new `query_sessions` row with `source='schedule'`, `user_id`, `conversation_id`, and `query=prompt`; (3) acquire (or create) the ACP session for `(user_id, conversation_id)` per the `chat-api-acp` contract; (4) submit the prompt via ACP `prompt`; (5) stream the assistant response back through the same SSE / WS event channel that web clients of that conversation are subscribed to; (6) on success, set `last_fired_at` to now-ms, and if the row is one-shot, delete it; if daily and `repeat=0`, delete it; if daily and `repeat=1`, leave it for the next day.

#### Scenario: Daily 09:00 schedule fires at the right time

- **WHEN** a `chat_schedules` row exists with `hour=9, minute=0, repeat=1` and the ticker observes the current local time matches that hour/minute
- **THEN** the dispatcher submits the prompt to the conversation's ACP session and sets `last_fired_at`
- **AND** the schedule row is NOT deleted

#### Scenario: One-shot schedule fires once and is deleted

- **WHEN** a `chat_schedules` row exists with `one_shot_at <= now-ms`
- **THEN** the dispatcher fires the prompt and the row is deleted in the same transaction

#### Scenario: Daily non-repeat schedule fires once and is deleted

- **WHEN** a row with `hour, minute` set, `repeat=0` matches the current minute
- **THEN** the dispatcher fires the prompt and the row is deleted

#### Scenario: Disabled schedule is skipped

- **WHEN** a `chat_schedules` row has `enabled=0` and matches the time
- **THEN** the dispatcher SHALL skip it; `last_fired_at` is unchanged

#### Scenario: Schedule whose runtime is no longer registered is skipped, not deleted

- **WHEN** the conversation's `runtime` is not present in the runtime registry at fire time
- **THEN** the dispatcher logs a warning and skips this fire; the row remains so the operator can restore the runtime

### Requirement: Scheduled-fire reply is delivered to live web clients via existing SSE

When a scheduler dispatch produces assistant output for a conversation, any web client currently subscribed to that conversation's SSE / WebSocket stream SHALL receive the streamed chunks and the final `done` event in the same format as a manual prompt, with the `query_sessions.source='schedule'` flag visible in the persisted turn.

#### Scenario: Open chat tab shows the scheduled reply live

- **WHEN** the user has the conversation open in the browser and a schedule fires for it
- **THEN** the chat thread appends a new turn streamed from the ACP response, and the rendered turn carries the schedule indicator

#### Scenario: Scheduled reply is visible on next conversation load

- **WHEN** the user opens the conversation later
- **THEN** the message-list endpoint includes the scheduled turn with `source='schedule'`

### Requirement: Scheduler ticker remains the single fire source

The same `Scheduler.fireDue` ticker (default 30-second cadence) that handles legacy daily PTY/Discord jobs SHALL also handle `chat:<convID>` targets. On startup the scheduler SHALL load `chat_schedules` rows into its in-memory job map alongside `.perch/schedules.jsonl`.

#### Scenario: Single ticker, two job sources

- **WHEN** the perch process boots
- **THEN** the scheduler loads jobs from `.perch/schedules.jsonl` (legacy targets) and `chat_schedules` (new chat targets) into one job map; one ticker drives both

#### Scenario: Hot reload picks up new schedules

- **WHEN** an API request inserts a new `chat_schedules` row
- **THEN** the scheduler in-memory job map is updated within at most one ticker tick (≤30 seconds) without a process restart

### Requirement: Per-(user, conv) isolation prevents cross-firing

A `chat_schedules` row SHALL only ever be dispatched into the conversation whose id matches its `conversation_id` and only as the user whose id matches its `user_id`. The dispatcher SHALL NOT route the prompt anywhere else (no main-PTY fallback for `chat:` targets).

#### Scenario: chat: target never falls through to PTY

- **WHEN** a `chat:<convID>` job fires and the chat dispatch path returns an error
- **THEN** the scheduler SHALL log the failure and SHALL NOT write the prompt to the main PTY (no `pm.write` fallback)

#### Scenario: Two users with identical schedule prompts do not cross-leak

- **WHEN** user A and user B both have a daily 09:00 schedule against their own conversations
- **THEN** the dispatcher fires two independent ACP runs, one per (user, conv); neither user's response is visible to the other
