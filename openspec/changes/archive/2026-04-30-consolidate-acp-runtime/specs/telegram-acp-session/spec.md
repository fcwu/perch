## ADDED Requirements

### Requirement: Telegram messages route through ACP per-chat sessions

When perch receives a Telegram message that passes IM-allowlist validation, it SHALL acquire (or create) an ACP subprocess from the pool keyed by `telegram:chat:<chatID>`, submit the message as an ACP `prompt`, and reply to the same Telegram chat with the assistant's response.

#### Scenario: First message in a chat creates a new ACP session

- **WHEN** a Telegram message arrives in a chat that does not yet have a pooled subprocess
- **AND** the message passes the existing telegram allowlist / mention validation
- **THEN** perch starts a new `claude-agent-acp` subprocess
- **AND** runs ACP `initialize` + `new_session` with `permissionMode: "bypassPermissions"` and `workspace_path` set to the perch workspace
- **AND** issues `prompt(sessionID, messageText)` against the new session

#### Scenario: Subsequent messages reuse the chat's session

- **WHEN** another message arrives in the same chat while the pooled subprocess is alive
- **THEN** perch issues `prompt(sessionID, messageText)` directly
- **AND** Claude retains conversation context

#### Scenario: Telegram chat session subprocess crash auto-restarts

- **WHEN** the pooled subprocess for a chat exits unexpectedly
- **THEN** the next message in that chat triggers a new subprocess
- **AND** the previous conversation context is lost (best-effort restart, not state recovery)

### Requirement: Telegram replies are formatted from ACP output and sent to the originating chat

The accumulated text from `agent_message_chunk` SHALL be sent back to the same Telegram chat using the existing message-formatting rules (≤4096 byte chunks, code-block preservation), upon `RunCompleted`.

#### Scenario: Single-chunk reply

- **WHEN** the ACP run produces text fitting in one Telegram message (≤4096 bytes)
- **THEN** perch sends one `sendMessage` API call with the full reply

#### Scenario: Multi-chunk reply

- **WHEN** the ACP run produces text exceeding 4096 bytes
- **THEN** perch splits the text on safe boundaries (newline preferred) and sends multiple `sendMessage` API calls in order

#### Scenario: Empty output produces no Telegram message

- **WHEN** ACP `RunCompleted` arrives but the accumulated text is empty
- **THEN** perch does NOT send any reply (no empty message posted)

### Requirement: Telegram session pool honors idle timeout and per-user limits

The Telegram-specific subprocess pool entries SHALL share the global ACP session pool's idle timeout and per-user / global limit enforcement defined in `acp-client` capability.

#### Scenario: Idle timeout

- **WHEN** no message arrives in a Telegram chat for the configured idle window (default 15 minutes)
- **THEN** the pooled subprocess is terminated and removed from the pool
- **AND** the next message in that chat starts a fresh subprocess

### Requirement: Telegram does NOT use PTY or hooks

After this change, `im_telegram.go` SHALL NOT depend on `PTYManager`, SHALL NOT call `claude` CLI directly, and SHALL NOT receive `HookEvent` notifications.

#### Scenario: Telegram adapter no longer implements Notify(HookEvent)

- **WHEN** perch starts up
- **THEN** `TelegramAdapter` does not implement the legacy `Notify(HookEvent, lastText string) error` method
- **AND** the `IMAdapter` interface no longer declares this method

#### Scenario: Telegram adapter does not hold a PTY reference

- **WHEN** Telegram is initialized
- **THEN** `TelegramAdapter` does not contain a `*PTYManager` field
- **AND** does not call `s.pty.write(...)` for any inbound message
