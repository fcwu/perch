## Requirements
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

### Requirement: Discord image attachments are forwarded to ACP

When a Discord message arrives with image attachments (`ContentType` ∈ allow-list, `Size` ≤ limit), the Discord adapter SHALL fetch each attachment, base64-encode it, and include it as an `image` content block in the same ACP `prompt` as the message text.

#### Scenario: Image attachment + message text

- **WHEN** a Discord message arrives with `m.Content="diagnose this"` and one PNG attachment
- **THEN** the adapter calls `acp_session_pool.Acquire("discord:channel:<id>")` and issues `PromptWithContent(ctx, [{type:"text",text:"diagnose this"}, {type:"image",data:"<b64>",mimeType:"image/png"}], ...)` (flat ACP `ImageContent` schema)

#### Scenario: Attachment fetch failure falls back to text-only

- **WHEN** any attachment URL returns non-2xx or times out
- **THEN** the adapter sends the prompt with the remaining valid blocks (or text only) and appends `> 附件 <name> 下載失敗` to the final Discord reply

#### Scenario: Non-image attachments are silently dropped

- **WHEN** a Discord message has video/audio/document attachments mixed with valid images
- **THEN** the adapter SHALL include only the valid image attachments in the prompt; non-image attachments are dropped without error

#### Scenario: Server-side limits also apply to Discord

- **WHEN** a Discord message attaches more than `CHAT_UPLOAD_MAX_FILES` images, or any image exceeds `CHAT_UPLOAD_MAX_BYTES`
- **THEN** the adapter trims to the limit (oldest-kept) and logs a `slog.Warn`; the prompt is still sent with whatever fits

### Requirement: ACP run 圖片輸出以檔案附件方式傳送至 Discord

當 Discord session 的 ACP run 完成並帶有圖片附件時，Perch 應將這些圖片以 Discord 檔案附件方式，與文字回覆同一則訊息一起傳送。

#### Scenario: 圖片與文字同一訊息傳送

- **WHEN** `RunCompleted` 為 Discord session 產生圖片附件記錄
- **THEN** Perch 讀取每張圖片檔案，加入 `MessageSend.Files`（`Name` 為原始檔名，`Reader` 為檔案位元組），並在同一個 `ChannelMessageSendComplex` 呼叫中與文字 chunk 一起送出

#### Scenario: 超過 Discord 8 MB 限制的圖片略過

- **WHEN** 圖片檔案大於 8 MB
- **THEN** 該檔案不加入 `MessageSend.Files`，並在最後一個文字 chunk 末尾附加 `(圖片過大，無法傳送至 Discord)`

#### Scenario: 圖片傳送失敗不影響文字回覆

- **WHEN** 某張圖片的 Discord 檔案上傳失敗
- **THEN** Perch 記錄錯誤、略過該圖片，仍正常傳送文字回覆與其他成功的圖片

#### Scenario: 無圖片時僅傳送文字回覆

- **WHEN** `RunCompleted` 未產生任何圖片附件記錄
- **THEN** Discord 回覆行為與原本相同（純文字訊息，套用現有 chunking 規則）

