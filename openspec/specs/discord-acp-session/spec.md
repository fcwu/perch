## ADDED Requirements

### Requirement: Discord message triggers an ACP run

When the Discord session receives a validated inbound Discord message, it SHALL trigger a new ACP `prompt` against the per-channel pooled session. ACP is the only supported runtime; there is no PTY fallback.

#### Scenario: Message creates ACP prompt

- **WHEN** a validated Discord message arrives
- **THEN** the Discord adapter calls `acp_session_pool.Acquire("discord:channel:<channelID>")` and issues `prompt(sessionID, messageContent)` with the channel ID as metadata

#### Scenario: ACP is the only path

- **WHEN** the perch container starts
- **THEN** Discord runtime does not check any `DISCORD_ACP_ENABLED` flag
- **AND** there is no code path that writes to a `*PTYManager` for Discord messages

### Requirement: Discord session reflects ACP run status with emoji reactions

While the ACP prompt is in progress, the session SHALL update Discord emoji reactions to indicate working state, and update them again upon completion.

#### Scenario: Run starts — eyes reaction added

- **WHEN** the ACP `prompt` is issued successfully
- **THEN** the 👀 reaction is added to the user's message

#### Scenario: Run completes — final reaction updated

- **WHEN** ACP `RunCompleted` arrives with success
- **THEN** the 👀 reaction is removed and 💬 is added

#### Scenario: Run fails — error reaction shown

- **WHEN** ACP `RunFailed` (or timeout) is reported
- **THEN** the 👀 reaction is removed and ❌ is added

> Note: the legacy ⚙️ (PreToolUse) and ✅ (PostToolUse) intermediate states are gone — Claude Code 2.1.x in ACP mode does not emit those tool hook events to perch.

### Requirement: ACP run output is sent to Discord as a formatted message

Upon ACP run completion, the accumulated text output SHALL be formatted and sent to the Discord channel using the existing output formatting rules.

#### Scenario: Output sent after run completion

- **WHEN** ACP `RunCompleted` arrives with success
- **THEN** the accumulated output text is split into ≤1900-character chunks and each chunk is sent as a Discord message to the originating channel

#### Scenario: Empty output handled gracefully

- **WHEN** the ACP run completes but returns no text output
- **THEN** no Discord message is sent (no empty message posted)

#### Scenario: Tables in output wrapped in code blocks

- **WHEN** the ACP run output contains table-formatted text
- **THEN** the output is wrapped in a code block with CJK-aware column alignment before sending to Discord

### Requirement: ACP run timeout is enforced per message

Each ACP run triggered by a Discord message SHALL be subject to a configurable timeout.

#### Scenario: Run completes within timeout

- **WHEN** the ACP run finishes before the timeout expires
- **THEN** the output is sent normally to Discord

#### Scenario: Run exceeds timeout

- **WHEN** the ACP run does not complete within the configured timeout
- **THEN** the session cancels the run, removes the 👀 reaction, adds ❌, and sends a timeout error message to Discord

### Requirement: Discord session pool conforms to shared ACP session pool

Discord per-channel sessions SHALL be managed by the shared `acp_session_pool` (capability `acp-client`), with key format `discord:channel:<channelID>`. They SHALL share idle timeout, per-user limits, and crash-restart behavior with chat-API and Telegram sessions.

#### Scenario: Shared pool key

- **WHEN** a Discord message arrives for channel `1234`
- **THEN** perch acquires the session via `acp_session_pool.Acquire("discord:channel:1234")`

#### Scenario: Pool eviction does not surprise Discord users

- **WHEN** a per-user or global pool limit forces an LRU eviction of a Discord channel session
- **THEN** the evicted session terminates cleanly
- **AND** the next Discord message in that channel transparently spawns a fresh subprocess (best-effort, not state recovery)
