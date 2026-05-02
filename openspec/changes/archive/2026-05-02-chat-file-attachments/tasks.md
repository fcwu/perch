## 1. Settings & limits

- [x] 1.1 Extend `defaultUploadAllowedMime` in `attachments.go` to include `text/plain`, `text/markdown`, `text/csv`, `text/x-log`, `application/json`, `application/x-ndjson`, `application/pdf`
- [x] 1.2 Add `UploadDirQuotaBytes *int64` field to `ChatSettings` (server.go) with env override `CHAT_UPLOAD_DIR_QUOTA_BYTES` (default 500 MiB) and admin UI binding in `frontend/src/SettingsPanel.tsx`
- [x] 1.3 Add `UploadOrphanTTLDays *int` field to `ChatSettings` with env override `CHAT_UPLOAD_ORPHAN_TTL_DAYS` (default 7)
- [x] 1.4 Extend `EffectiveAttachmentLimits` to surface the new quota + orphan TTL fields
- [x] 1.5 Update README/CLAUDE.md / docs section listing `CHAT_UPLOAD_*` env vars

## 2. MIME validation extensions

- [x] 2.1 Extend `MagicMime` to detect `application/pdf` via `%PDF-` prefix
- [x] 2.2 Add a `looksLikeText` helper (UTF-8 valid + control-char ratio < 1% on first 8 KiB) covering text/plain, text/markdown, text/csv, text/x-log, application/json, application/x-ndjson; for these MIMEs, accept client-claimed type when the heuristic passes
- [x] 2.3 Add unit tests in `attachments_test.go` for: PDF magic match, text heuristic accept (csv/json/log/md), text heuristic reject (binary masquerading as text), client-claimed/magic mismatch still rejected for image MIMEs (added in `attachments_disk_test.go`)

## 3. Filename sanitization & path safety

- [x] 3.1 Implement `SanitizeAttachmentFilename(name string) (string, error)`: strip path separators, NUL, `..` segments; reject empty result; cap to 200 chars (preserving extension); allow ASCII alphanumeric + common punctuation + Unicode letters (CJK ok)
- [x] 3.2 Implement `ResolveAttachmentPath(workdir, convID, sanitizedName) (string, error)`: build path, run `filepath.Clean`, verify result is still under `<workdir>/uploads/<convID>/`; reject otherwise
- [x] 3.3 Implement `NextAvailableFilename(dir, sanitizedName) string`: if `dir/sanitizedName` exists, append ` (2)`, ` (3)`, … before extension until free
- [x] 3.4 Unit tests in `attachments_test.go` covering: traversal (`../etc/passwd`), embedded `/`, NUL byte, length cap, CJK pass-through, dup-name `(2)` suffix

## 4. Disk-write path

- [x] 4.1 Add `WriteAttachmentsToDisk(ctx, workdir, convID, atts []Attachment, lim AttachmentLimits) ([]PersistedAttachment, error)` in `attachments.go`: only handles non-image MIMEs; mkdir-p the conv dir; check quota before writing (sum existing dir bytes + new sizes); write files all-or-nothing (use temp file + rename, cleanup on any error); return slice of `{Filename, RelPath, MimeType, SizeBytes}`
- [x] 4.2 Add `BuildPromptFilePrefix(persisted []PersistedAttachment, userText string) string`: produces `[file: <relpath> (<mime>, <human-size>)]` lines (one per persisted file) + blank line + userText; if userText is empty, no trailing blank line
- [x] 4.3 Add `humanSize(n int64) string` (B / KiB / MiB / GiB, one decimal above KiB)
- [x] 4.4 Unit tests for `WriteAttachmentsToDisk`: success path, quota exceeded rolls back, partial write cleanup on second-file failure, image MIME bypass (image returns empty persisted slice)
- [x] 4.5 Unit tests for `BuildPromptFilePrefix`: 1 file + text, multiple files + text, files only no text, image-only no prefix

## 5. Chat-API integration

- [x] 5.1 In `chat_api_acp.go`, after `ValidateAttachments`, partition attachments by `isImageMIME(mime)` into `images` + `nonImages`
- [x] 5.2 Call `WriteAttachmentsToDisk(ctx, workdir, conversationID, nonImages, lim)` and capture `persisted`
- [x] 5.3 Build the text content block via `BuildPromptFilePrefix(persisted, queryText)`; build image content blocks via existing `AttachmentsToACPBlocks(images)`; concatenate `[textBlock, imageBlocks...]` and pass to `PromptWithContent`
- [x] 5.4 Update the `query_sessions.query` placeholder to combine image + file markers: `[image:foo.png] [file:bar.pdf] <orig>` (extend the existing image-only placeholder builder)
- [x] 5.5 On request reject (any validation / quota / disk error), ensure no partial files remain (rely on `WriteAttachmentsToDisk` rollback; verify chat-api wrapper does not write before validation)

- [x] 6.1 In `user_session.go` (or wherever the ACP session pool evicts entries), add a hook that runs `os.RemoveAll(<workdir>/uploads/<convID>)` after the subprocess has exited; log warn on error, continue
- [x] 6.2 In `bootstrap.go` (or `main.go` startup path), implement `cleanupOrphanUploads(workdir string, ttl time.Duration)`: list `<workdir>/uploads/*`, walk each conv-dir to find the latest mtime, `os.RemoveAll` directories whose latest mtime is older than `ttl`
- [x] 6.3 Wire `cleanupOrphanUploads` into perch startup (after store is open, before serving requests); log `info` with kept/removed counts

## 6. Per-conversation directory lifecycle

- [x] 6.4 Add unit test for `cleanupOrphanUploads` using a tmp dir: stale-mtime dir removed, fresh-mtime dir kept, missing uploads root is no-op (fail open, never destructive)

## 7. Discord adapter

- [x] 7.1 In `im_discord.go`, change the inbound attachment loop: keep image classification path; for non-image (`ContentType` ∈ allow-list excluding image MIMEs), fetch the URL and feed bytes into the same `WriteAttachmentsToDisk` flow under conv key `<channelID>` for the per-conversation uploads dir
- [x] 7.2 Compose the prompt: text block via `BuildPromptFilePrefix(persistedNonImages, m.Content)`, plus image blocks; pass to `PromptWithContent`
- [x] 7.3 On disallowed MIME, append `> 附件 <name> 不支援此類型 (<mime>)` to the Discord reply (replaces the prior silent-drop)
- [x] 7.4 On fetch failure for non-image, append `> 附件 <name> 下載失敗` (same message as image) and ensure no partial file remains
- [x] 7.5 Update `im_discord_acp_test.go`: add cases for non-image attachment persisted + prompt prefix, mixed image+file, disallowed MIME user note (added in `im_discord_attach_test.go`)

## 8. Frontend

- [x] 8.1 In `frontend/src/ChatPage.tsx`, change `ALLOWED_UPLOAD_MIME` to the extended set (mirror server allow-list); update `<input type=file accept>` to the same set
- [x] 8.2 Update `PendingAttachment` type / chip rendering to distinguish image (thumbnail) vs file (icon + filename + human size); add a generic file icon SVG
- [x] 8.3 Update placeholder text generation (`[image:filename]` vs `[file:filename]`) used in the optimistic local user-message render so it matches the server-side placeholder
- [x] 8.4 Update drag-drop placeholder text from "Drop images here…" to "Drop files here…"
- [x] 8.5 Add an inline error toast if user picks a file whose MIME is not in the new allow-list (client-side pre-check)

## 9. Tests / e2e

- [x] 9.1 Create `tests/test-chat-file-attach.md` with cases CFA01–CFA08:
  - CFA01: `/api/chat` with one PDF persists to disk + prompt prefix correct
  - CFA02: `/api/chat` with one CSV (text MIME via heuristic) persists + prefix correct
  - CFA03: `/api/chat` with mixed PNG + PDF: PNG inline, PDF persisted + prefix
  - CFA04: `/api/chat` with same-named CSV twice in same conv → second writes as `name (2).csv`
  - CFA05: `/api/chat` request that would exceed `CHAT_UPLOAD_DIR_QUOTA_BYTES` → 400, no disk write
  - CFA06: ACP pool eviction (idle timeout simulated) → conv uploads dir removed
  - CFA07: perch restart with orphan dir present (conv-id not in store) → orphan removed, recent dir kept
  - CFA08: Discord message with PDF attachment → persisted under `discord:channel:<id>` + prompt prefix
- [x] 9.2 Run `go vet ./... && go test ./...` and ensure all unit tests pass (vet has only the pre-existing `acp_process_test.go:436` warning unrelated to this change)
- [x] 9.3 Run `cd frontend && npm run build && npm run lint` to verify frontend compiles (`npm run build` succeeds; lint script not configured)
- [x] 9.4 Run `tests/test-chat-file-attach.md` cases against a deployed instance per `tests/.env.<device>.md` — QA pass 2026-05-02 covered CFA01/02/03/04/05/07 + CU01/02/03/04 regression + post-fix re-run; CFA06 skipped pending pool-evict trigger gap, CFA08 manual due to chrome-cdp + Slate-editor limitation

## 10. Docs & cleanup

- [x] 10.1 Update `README.md` chat-attachment section to document non-image upload behavior, the `[file: ...]` prefix convention, and the new `CHAT_UPLOAD_*` env vars
- [x] 10.2 Add a short note in `CLAUDE.md` (or perch `CLAUDE.md`) about the `<workdir>/uploads/<conv-id>/` layout for agent authors
- [x] 10.3 Verify backwards compat: QA 2026-05-02 confirmed image-only requests do NOT create `uploads/`, history placeholder still `[image:filename]`, all CU01–CU05 image-only rejection paths still return correct 400 messages — rollback to image-only allow-list is just a settings change, no schema needed
- [x] 10.4 Run `openspec validate chat-file-attachments` and address any findings before requesting archive
