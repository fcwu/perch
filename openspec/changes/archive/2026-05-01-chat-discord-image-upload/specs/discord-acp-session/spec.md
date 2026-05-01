## ADDED Requirements

### Requirement: Discord image attachments are forwarded to ACP

When a Discord message arrives with image attachments (`ContentType` ∈ allow-list, `Size` ≤ limit), the Discord adapter SHALL fetch each attachment, base64-encode it, and include it as an `image` content block in the same ACP `prompt` as the message text.

#### Scenario: Image attachment + message text

- **WHEN** a Discord message arrives with `m.Content="diagnose this"` and one PNG attachment
- **THEN** the adapter calls `acp_session_pool.Acquire("discord:channel:<id>")` and issues `PromptWithContent(ctx, [{type:"text",text:"diagnose this"}, {type:"image",source:{type:"base64",media_type:"image/png",data:"<b64>"}}], ...)`

#### Scenario: Attachment fetch failure falls back to text-only

- **WHEN** any attachment URL returns non-2xx or times out
- **THEN** the adapter sends the prompt with the remaining valid blocks (or text only) and appends `> 附件 <name> 下載失敗` to the final Discord reply

#### Scenario: Non-image attachments are silently dropped

- **WHEN** a Discord message has video/audio/document attachments mixed with valid images
- **THEN** the adapter SHALL include only the valid image attachments in the prompt; non-image attachments are dropped without error

#### Scenario: Server-side limits also apply to Discord

- **WHEN** a Discord message attaches more than `CHAT_UPLOAD_MAX_FILES` images, or any image exceeds `CHAT_UPLOAD_MAX_BYTES`
- **THEN** the adapter trims to the limit (oldest-kept) and logs a `slog.Warn`; the prompt is still sent with whatever fits
