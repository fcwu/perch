## MODIFIED Requirements

### Requirement: Chat-API accepts image attachments alongside the text query

`POST /api/chat` SHALL accept an optional `attachments` array of `{filename, mime_type, data_base64}` objects. The server SHALL classify each attachment by `mime_type`:

- **Image MIME** (`image/png`, `image/jpeg`, `image/gif`, `image/webp`): forwarded to ACP as `image` content blocks in the same `session/prompt` call as the text query (unchanged from prior behavior).
- **Non-image MIME** (text/*, application/pdf, application/json, application/x-ndjson, etc., as configured in `CHAT_UPLOAD_ALLOWED_MIME`): persisted to disk under `<workdir>/uploads/<conversation_id>/<sanitized_filename>` and referenced by a `[file: ...]` prefix line prepended to the text content block.

A single request MAY mix image and non-image attachments; the server SHALL apply the appropriate path per attachment.

#### Scenario: Pure-text query is unchanged

- **WHEN** the client posts `{"query":"hi","new_conversation":true}` (no `attachments` field)
- **THEN** the server SHALL behave exactly as before — single text content block, no validation overhead, no disk write

#### Scenario: Query with one image is forwarded to ACP as image block

- **WHEN** the client posts `{"query":"what's wrong here?","attachments":[{"filename":"err.png","mime_type":"image/png","data_base64":"<b64>"}]}`
- **THEN** the server SHALL call `PromptWithContent(ctx, [{type:"text",text:"what's wrong here?"}, {type:"image",data:"<b64>",mimeType:"image/png"}], ...)` (flat ACP `ImageContent` schema, NOT Anthropic-style nested `source`)
- **AND** the server SHALL NOT write any bytes under `<workdir>/uploads/`

#### Scenario: Query with one non-image file is persisted to disk and referenced in prompt

- **WHEN** the client posts `{"query":"summarize the spec","attachments":[{"filename":"spec.pdf","mime_type":"application/pdf","data_base64":"<b64>"}]}` for conversation `c-abc123`
- **THEN** the server SHALL write the decoded bytes to `<workdir>/uploads/c-abc123/spec.pdf`
- **AND** the server SHALL call `PromptWithContent(ctx, [{type:"text",text:"[file: ./uploads/c-abc123/spec.pdf (application/pdf, 1.2 MiB)]\n\nsummarize the spec"}], ...)` (single text block; no image block; relative path uses `./uploads/...` form)
- **AND** the file SHALL remain on disk for the duration of the conversation (not deleted at prompt end)

#### Scenario: Query with mixed image and non-image attachments

- **WHEN** the client posts a request with one `image/png` and one `application/pdf` attachment
- **THEN** the server SHALL persist the PDF to disk and prepend `[file: ...]` to the text
- **AND** the server SHALL include the image as an `image` content block alongside the text block in the same `session/prompt` call
- **AND** the prompt blocks order SHALL be: `[text-with-file-prefix, image]`

#### Scenario: Server validates MIME, size, count, and per-conversation quota

- **WHEN** the server receives `attachments`
- **THEN** the server SHALL reject (HTTP 400) when:
  - any attachment's `mime_type` is not in `CHAT_UPLOAD_ALLOWED_MIME` (default extended to: `image/png,image/jpeg,image/gif,image/webp,text/plain,text/markdown,text/csv,text/x-log,application/json,application/x-ndjson,application/pdf`)
  - any attachment's decoded byte size exceeds `CHAT_UPLOAD_MAX_BYTES` (default: 10 MB)
  - the magic bytes of decoded data do not match the claimed `mime_type` (image MIMEs and `application/pdf` use exact magic-byte match; text/* MIMEs use a UTF-8-validity + printable-ratio heuristic)
  - `len(attachments) > CHAT_UPLOAD_MAX_FILES` (default: 4)
  - persisting the new non-image attachments would push `<workdir>/uploads/<conversation_id>/` total bytes over `CHAT_UPLOAD_DIR_QUOTA_BYTES` (default: 500 MB)
- **AND** when rejected, no partial writes SHALL remain on disk (write all-or-nothing per request)

#### Scenario: Filename sanitization prevents path traversal

- **WHEN** an attachment has filename `../../etc/passwd` or `foo/bar.txt` or contains NUL bytes
- **THEN** the server SHALL either reject the request (HTTP 400) or sanitize the filename by stripping path separators and `..` segments before writing
- **AND** the resolved write path SHALL be verified (via `filepath.Clean` + prefix check) to stay within `<workdir>/uploads/<conversation_id>/`; any path that escapes SHALL be rejected

#### Scenario: Duplicate filename within a conversation gets a numeric suffix

- **WHEN** the conversation `c-abc123` already has `<workdir>/uploads/c-abc123/error.log` and a new request uploads another `error.log`
- **THEN** the server SHALL write the new file as `<workdir>/uploads/c-abc123/error.log (2)` (or next available `(N)` suffix)
- **AND** the prompt prefix SHALL reference the suffixed name: `[file: ./uploads/c-abc123/error.log (2) (text/x-log, ...)]`

#### Scenario: query field shows attachment placeholder in management history

- **WHEN** a chat-API query with attachments completes
- **THEN** `query_sessions.query` SHALL contain placeholder-prefixed form combining both image and file markers, e.g. `[image:err.png] [file:spec.pdf] <original text>`, so the management history list does not embed base64 data or file paths

#### Scenario: Image attachment bytes are not persisted

- **WHEN** the ACP run completes for a request that contained image attachments only
- **THEN** the server SHALL NOT write image bytes to `/data`, the workspace, `<workdir>/uploads/`, or `query_log_store`; image bytes live only in process memory for the duration of the prompt

## ADDED Requirements

### Requirement: Per-conversation upload directory lifecycle

Each conversation's persisted uploads SHALL live in `<workdir>/uploads/<conversation_id>/`. The server SHALL clean up these directories when the conversation is no longer in use, so disk usage cannot grow unbounded.

#### Scenario: Directory is created lazily on first non-image upload

- **WHEN** a request for conversation `c-abc123` posts the first non-image attachment
- **THEN** the server SHALL create `<workdir>/uploads/c-abc123/` (mkdir-p) before writing any file
- **AND** the directory SHALL have permissions readable by the agent subprocess (`0755` or compatible with the runtime user)

#### Scenario: Directory is removed when ACP session is evicted from pool

- **WHEN** the ACP session pool removes a `(user_id, conversation_id)` entry due to idle timeout, LRU eviction, or subprocess crash
- **THEN** the server SHALL recursively delete `<workdir>/uploads/<conversation_id>/` after the subprocess has exited
- **AND** if the deletion fails, the server SHALL log a warning but not block other operations

#### Scenario: Startup scan removes orphan directories by mtime

- **WHEN** perch starts up
- **THEN** the server SHALL list all `<workdir>/uploads/<conv-id>/` directories
- **AND** for each directory whose latest file mtime is older than `CHAT_UPLOAD_ORPHAN_TTL_DAYS` (default 7) days, the server SHALL recursively delete the directory
- **AND** the scan result (count kept, count removed) SHALL be logged at info level
- **AND** mtime is used (rather than checking against the store) so Discord channel directories — which are not tracked in `query_sessions` — are retained while in use

#### Scenario: Read-only `<workdir>` skips disk-save and rejects non-image uploads

- **WHEN** `<workdir>` is not writable (e.g., misconfigured volume) at request time
- **THEN** the server SHALL reject any request with non-image attachments (HTTP 500 with a clear error message)
- **AND** image-only requests SHALL still succeed (no disk write needed)

### Requirement: Prompt prefix format for persisted file attachments

For each non-image attachment in a request, the server SHALL prepend a `[file: ...]` line to the user's text content. The format SHALL be deterministic so agents can reliably parse and reference the file path.

#### Scenario: Single non-image attachment prefix

- **WHEN** a request has one attachment `error.log` (text/x-log, 142 KiB) for conversation `c-abc123` with text `"check this"`
- **THEN** the resulting text content block SHALL be exactly:
  ```
  [file: ./uploads/c-abc123/error.log (text/x-log, 142 KiB)]

  check this
  ```
  (one prefix line, one blank line, then the original text)

#### Scenario: Multiple non-image attachments

- **WHEN** a request has two non-image attachments `a.csv` and `b.pdf`
- **THEN** the resulting text content block SHALL have two `[file: ...]` lines in attachment order, then one blank line, then the original text
- **AND** image attachments SHALL NOT contribute `[file: ...]` lines

#### Scenario: Empty user text with attachments

- **WHEN** a request has at least one non-image attachment and `query` is empty/whitespace
- **THEN** the resulting text content block SHALL be the `[file: ...]` line(s) only (no trailing blank line + empty body)
- **AND** the request SHALL NOT be rejected as empty (the file prefix counts as content)

#### Scenario: Size formatting is human-readable

- **WHEN** the server formats the size in `[file: ...]`
- **THEN** sizes SHALL use binary units: `B` for < 1024, `KiB` for < 1 MiB, `MiB` for < 1 GiB, `GiB` otherwise, with one decimal place above `KiB`
