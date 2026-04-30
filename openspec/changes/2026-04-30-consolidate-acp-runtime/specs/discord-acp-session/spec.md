## REMOVED Requirements

### Requirement: Discord session falls back to PTY mode when ACP is disabled

**Reason for removal**: ACP is now the only supported runtime for Discord; the PTY fallback path is removed.

> Original capability text: this Requirement permitted `DISCORD_ACP_ENABLED=false` to fall back to PTY-based Discord sessions. With this change, the environment variable is removed and the Discord adapter has no PTY code path.

## MODIFIED Requirements

### Requirement: Discord message triggers an ACP run

When the Discord session receives a validated inbound Discord message, it SHALL trigger a new ACP `prompt` against the per-channel pooled session. (The original Requirement's conditional clause "when the Discord session is in ACP mode" is removed because there is no other mode.)

#### Scenario: Message creates ACP prompt

- **WHEN** a validated Discord message arrives
- **THEN** the Discord adapter calls `acp_session_pool.Acquire("discord:channel:<channelID>")` and issues `prompt(sessionID, messageContent)` with the channel ID as metadata

#### Scenario: ACP is the only path

- **WHEN** the perch container starts
- **THEN** Discord runtime does not check any `DISCORD_ACP_ENABLED` flag
- **AND** there is no code path that writes to a `*PTYManager` for Discord messages

### Requirement: Discord session reflects ACP run status with emoji reactions

While the ACP prompt is in progress, the session SHALL update Discord emoji reactions to indicate working state, and update them again upon completion. *(Unchanged from prior spec; restated here only because the parent Requirement was modified.)*

#### Scenario: Run starts — eyes reaction added

- **WHEN** the ACP `prompt` is issued successfully
- **THEN** the 👀 reaction is added to the user's message

#### Scenario: Run completes — final reaction updated

- **WHEN** ACP `RunCompleted` arrives with success
- **THEN** the 👀 reaction is removed and 💬 is added

#### Scenario: Run fails — error reaction shown

- **WHEN** ACP `RunFailed` (or timeout) is reported
- **THEN** the 👀 reaction is removed and ❌ is added

## ADDED Requirements

### Requirement: Discord session pool conforms to shared ACP session pool

Discord per-channel sessions SHALL be managed by the shared `acp_session_pool` (capability `acp-client`), with key format `discord:channel:<channelID>`. They SHALL share idle timeout, per-user limits, and crash-restart behavior with chat-API and Telegram sessions.

#### Scenario: Shared pool key

- **WHEN** a Discord message arrives for channel `1234`
- **THEN** perch acquires the session via `acp_session_pool.Acquire("discord:channel:1234")`

#### Scenario: Pool eviction does not surprise Discord users

- **WHEN** a per-user or global pool limit forces an LRU eviction of a Discord channel session
- **THEN** the evicted session terminates cleanly
- **AND** the next Discord message in that channel transparently spawns a fresh subprocess (best-effort, not state recovery)
