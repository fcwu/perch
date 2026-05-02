## MODIFIED Requirements

### Requirement: Discord image attachments are forwarded to ACP

When a Discord message arrives with attachments (`ContentType` ∈ allow-list, `Size` ≤ limit), the Discord adapter SHALL classify each attachment by content type:

- **Image attachments**: fetched, base64-encoded, and included as `image` content blocks in the same ACP `prompt` as the message text (unchanged from prior behavior).
- **Non-image attachments** (text/*, application/pdf, application/json, etc., per `CHAT_UPLOAD_ALLOWED_MIME`): fetched and persisted to `<workdir>/uploads/<conversation_id>/<sanitized_filename>`, where `<conversation_id>` is the Discord channel/thread-derived conversation key (`discord:channel:<id>` or equivalent). A `[file: ...]` prefix line SHALL be prepended to the message text content block.

This MODIFIES the prior behavior where non-image attachments were silently dropped.

#### Scenario: Image attachment + message text

- **WHEN** a Discord message arrives with `m.Content="diagnose this"` and one PNG attachment
- **THEN** the adapter calls `acp_session_pool.Acquire("discord:channel:<id>")` and issues `PromptWithContent(ctx, [{type:"text",text:"diagnose this"}, {type:"image",data:"<b64>",mimeType:"image/png"}], ...)` (flat ACP `ImageContent` schema)
- **AND** no file is written under `<workdir>/uploads/`

#### Scenario: Non-image attachment is persisted and referenced

- **WHEN** a Discord message arrives with `m.Content="check this log"` and one `error.log` attachment (`ContentType="text/x-log"`)
- **THEN** the adapter SHALL fetch the attachment URL, validate MIME / size / quota (same rules as `/api/chat`), and write to `<workdir>/uploads/discord:channel:<id>/error.log`
- **AND** the adapter SHALL issue `PromptWithContent(ctx, [{type:"text",text:"[file: ./uploads/discord:channel:<id>/error.log (text/x-log, 142 KiB)]\n\ncheck this log"}], ...)` (single text block with prefix)

#### Scenario: Mixed image and non-image attachments

- **WHEN** a Discord message has one PNG and one PDF attachment
- **THEN** the adapter SHALL persist the PDF, prepend `[file: ...]` to the text, and include the PNG as an `image` block in the same prompt

#### Scenario: Attachment fetch failure falls back gracefully

- **WHEN** any attachment URL returns non-2xx or times out
- **THEN** the adapter SHALL send the prompt with the remaining valid blocks (text + whichever attachments succeeded) and append `> 附件 <name> 下載失敗` to the final Discord reply
- **AND** any partial files written for the failed batch SHALL be cleaned up (no orphan partial bytes on disk)

#### Scenario: Server-side limits also apply to Discord

- **WHEN** a Discord message attaches more than `CHAT_UPLOAD_MAX_FILES` items, or any item exceeds `CHAT_UPLOAD_MAX_BYTES`, or the per-conversation directory quota would be exceeded
- **THEN** the adapter SHALL trim or reject (logging a `slog.Warn`) and send the prompt with whatever fits within limits; the user SHALL receive a `> 附件 <name>: <reason>` note appended to the Discord reply explaining what was dropped

#### Scenario: Disallowed MIME is dropped with a user-visible note

- **WHEN** a Discord message has an attachment whose `ContentType` is not in `CHAT_UPLOAD_ALLOWED_MIME` (e.g., `video/mp4` in Phase 1)
- **THEN** the adapter SHALL drop the attachment, log a `slog.Warn`, and append `> 附件 <name> 不支援此類型 (<mime>)` to the Discord reply
