## ADDED Requirements

### Requirement: Hook endpoint accepts Claude Code events
Perch SHALL expose `POST /hook` that accepts Claude Code Hook JSON payloads and dispatches events to all active IM adapters.

#### Scenario: Valid hook event received
- **WHEN** a POST request is sent to `/hook` with a valid JSON body containing a `hook_event_name` field
- **THEN** the endpoint returns HTTP 200
- **THEN** the event is dispatched to all active IM adapters

#### Scenario: Invalid JSON rejected
- **WHEN** a POST request is sent to `/hook` with invalid JSON
- **THEN** the endpoint returns HTTP 400

### Requirement: Discord reactions reflect Claude execution state
When a Discord message is being processed, Perch SHALL add emoji reactions to the original message to reflect Claude's execution state.

#### Scenario: PreToolUse event
- **WHEN** a `PreToolUse` hook event is received
- **WHEN** a Discord message is pending (lastMsg exists)
- **THEN** a ⚙️ reaction is added to the original Discord message

#### Scenario: PostToolUse success event
- **WHEN** a `PostToolUse` hook event is received with no error
- **WHEN** a Discord message is pending
- **THEN** a ✅ reaction is added to the original Discord message
- **THEN** the ⚙️ reaction is removed

#### Scenario: PostToolUse failure event
- **WHEN** a `PostToolUse` hook event is received with an error
- **WHEN** a Discord message is pending
- **THEN** a ❌ reaction is added to the original Discord message
- **THEN** the ⚙️ reaction is removed

#### Scenario: Stop event — Discord
- **WHEN** a `Stop` hook event is received
- **WHEN** a Discord message is pending
- **THEN** a 💬 reaction is added to the original Discord message
- **THEN** the 👀 reaction is removed
- **THEN** a reply message is sent to the same channel
- **THEN** the pending Discord message is cleared

#### Scenario: Reaction failure does not crash
- **WHEN** adding a Discord reaction fails (e.g., rate limit)
- **THEN** the error is logged as a warning
- **THEN** Perch continues running normally

### Requirement: Telegram response sent on Stop
When a Telegram message is being processed, Perch SHALL send a text reply when Claude finishes.

#### Scenario: Stop event — Telegram
- **WHEN** a `Stop` hook event is received
- **WHEN** a Telegram message is pending
- **THEN** a reply is sent to the original Telegram chat
- **THEN** the pending Telegram message is cleared

### Requirement: Hook settings baked into container image
The Claude Code Hook configuration SHALL be included in the Docker image at `/root/.claude/settings.json` so that hooks activate automatically without user configuration.

#### Scenario: Hook fires on tool use
- **WHEN** Claude Code executes a tool inside the container
- **THEN** `POST /hook` is called with the tool event payload
