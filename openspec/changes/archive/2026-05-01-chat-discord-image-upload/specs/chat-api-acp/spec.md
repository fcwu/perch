## ADDED Requirements

### Requirement: Chat-API accepts image attachments alongside the text query

`POST /api/chat` SHALL accept an optional `attachments` array of `{filename, mime_type, data_base64}` objects. The server SHALL forward validated attachments to ACP as `image` content blocks in the same `session/prompt` call as the text query.

#### Scenario: Pure-text query is unchanged

- **WHEN** the client posts `{"query":"hi","new_conversation":true}` (no `attachments` field)
- **THEN** the server SHALL behave exactly as before — single text content block, no validation overhead

#### Scenario: Query with one image is forwarded to ACP

- **WHEN** the client posts `{"query":"what's wrong here?","attachments":[{"filename":"err.png","mime_type":"image/png","data_base64":"<b64>"}]}`
- **THEN** the server SHALL call `PromptWithContent(ctx, [{type:"text",text:"what's wrong here?"}, {type:"image",data:"<b64>",mimeType:"image/png"}], ...)` (flat ACP `ImageContent` schema, NOT Anthropic-style nested `source`)

#### Scenario: Server validates MIME, size, and count

- **WHEN** the server receives `attachments`
- **THEN** the server SHALL reject (HTTP 400) when:
  - any attachment's `mime_type` is not in `CHAT_UPLOAD_ALLOWED_MIME` (default: `image/png,image/jpeg,image/gif,image/webp`)
  - any attachment's decoded byte size exceeds `CHAT_UPLOAD_MAX_BYTES` (default: 10 MB)
  - the magic bytes of decoded data do not match the claimed `mime_type`
  - `len(attachments) > CHAT_UPLOAD_MAX_FILES` (default: 4)

#### Scenario: query field shows attachment placeholder in management history

- **WHEN** a chat-API query with attachments completes
- **THEN** `query_sessions.query` SHALL contain the placeholder-prefixed form `[image:<filename1>] [image:<filename2>] <original text>` so the management history list does not embed base64 data

#### Scenario: Attachments are not persisted

- **WHEN** the ACP run completes
- **THEN** the server SHALL NOT write attachment bytes to `/data`, the workspace, or `query_log_store`; the attachment bytes live only in process memory for the duration of the prompt
