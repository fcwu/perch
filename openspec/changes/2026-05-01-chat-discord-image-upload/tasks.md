## 0. Pre-flight

- [ ] 0.1 驗證 claude-agent-acp 2.x 對 `session/prompt` image content block 的支援：手動跑一個含 image 的 prompt，確認 RunCompleted 不報錯。記下實測 ACP 版本與 Claude model — **由 Phase 8.2 QA 涵蓋**（CU01 真實 ACP run 帶 PNG，PASS = 確認支援）
- [x] 0.2 解 Open Questions Q1-Q4（見 design.md）— Q1 (允許 image+tool，Phase 0 實測 confirm) / Q2 (Discord 非圖 attachment 靜默 drop) / Q3 (image 不入 tool_events) / Q4 (反向不在本 change) 全部按 design.md 預設值定案

## 1. 共用：ACP content blocks 抽象

- [x] 1.1 在 `acp_process.go` 新增 `ACPContent` + `ACPImageSource` struct
- [x] 1.2 新增 `func (p *ACPProcess) PromptWithContent(ctx, blocks, ...)`
- [x] 1.3 `PromptWithChunks(text, ...)` 包成單一 text block 後呼叫 `PromptWithContent`
- [x] 1.4 `acp_process_test.go::TestACPProcess_PromptWithContent_ImageBlock` 驗 [text, image] 兩 block 序列化正確

## 2. Server-side validation 共用 helper

- [x] 2.1 新增 `attachments.go`：`Attachment` / `AttachmentLimits` struct + `ValidateAttachments` + `MagicMime`（PNG/JPEG/GIF/WEBP）+ `AttachmentsToACPBlocks` + `EffectiveAttachmentLimits`
- [x] 2.2 `settings.go` 加 `ChatSettings`（`upload_max_bytes` / `upload_max_files` / `upload_allowed_mime`）+ mergeSettings + ApplyDelta（皆為 immediate，不 set dirty）；`main.go::buildEnvSeed` 讀 `CHAT_UPLOAD_MAX_BYTES`、`CHAT_UPLOAD_MAX_FILES`、`CHAT_UPLOAD_ALLOWED_MIME`，預設 10MB / 4 / `image/png,image/jpeg,image/gif,image/webp`
- [x] 2.3 `attachments_test.go` 14 case：magic 4 種 + unknown、empty atts、happy path、dataURI prefix strip、TooMany、DisallowedMime、Oversized、MagicMismatch、InvalidBase64、ToACPBlocks、EffectiveLimits defaults/override

## 3. Chat-API：accept attachments

- [x] 3.1 `POST /api/chat` request struct 加 `Attachments []Attachment`
- [x] 3.2 `ChatSessionManager.StartSession` 介面 + `ACPUserSessionManager.StartSession` + `UserSessionManager.StartSession`（legacy 略過）簽名擴充 `attachments []Attachment`；handler 端先跑 `ValidateAttachments`，失敗回 400 + JSON error
- [x] 3.3 `chat_api_acp.go::runPrompt` 拼 ACP content blocks `[text(query), AttachmentsToACPBlocks(...)...]` 然後呼叫 `PromptWithContent`
- [x] 3.4 ManagementHub `SessionAdded` / `query_log_store.InsertSession` 寫入 `[image:foo.png] [image:bar.jpg] <原始 text>` placeholder（`formatQueryForHistory`）
- [x] 3.5 → CU01 in test-chat-upload.md
- [x] 3.6 → CU02 in test-chat-upload.md

## 4. Chat UI：upload entrypoint

- [x] 4.1 PaperclipIcon button + hidden `<input type="file" accept="image/*" multiple>`
- [x] 4.2 dropzone：onDragEnter/Over/Leave/Drop 在 textarea wrapper 上；dragOver 時 border 改藍 + placeholder 改 `Drop images here…`
- [x] 4.3 onPaste：抓 `clipboardData.items` 的 file kind，無檔名時 fabricate `pasted-<timestamp>.png`
- [x] 4.4 chip 列：`📎 filename size ✕`；✕ 呼叫 `removeAttachment(idx)`
- [x] 4.5 `fileToAttachment`：`File.arrayBuffer()` → `btoa(binary)` → `attachments` array 進 body
- [x] 4.6 送出後 `setAttachments([])`；HTTP 非 200 顯示 alert
- [x] 4.7 → CU03 in test-chat-upload.md
- [x] 4.8 → CU04 in test-chat-upload.md

## 5. Discord：fetch + forward attachments

- [x] 5.1 `im_discord.go::fetchImageAttachments` 取 `m.Attachments`，allow-list/size/count 全 server-side 過濾 + magic byte 重檢
- [x] 5.2 `fetchURLBytes` 對每個過濾後 attachment HTTP GET（30s timeout、`io.LimitReader` 防 DoS）→ base64
- [x] 5.3 `handleWithACP` 拼 `[text, image…]` blocks 呼叫 `PromptWithContent`；text 為空時補空字串維持 ACP 至少一塊規則
- [x] 5.4 fetch 失敗：`failed` filename 列表附在 reply 末尾 `> 附件 X 下載失敗，未送進 Claude`
- [x] 5.5 非圖片 attachment（不在 `AllowedMime`）silent drop
- [x] 5.6 → CU05 in test-chat-upload.md
- [x] 5.7 → CU06 in test-chat-upload.md

## 6. 觀測 / 限速

- [x] 6.1 `chat_api_acp.go::StartSession` log `count` + `total_b64_bytes`；`im_discord.go::fetchImageAttachments` log `kept` + `failed`
- [x] 6.2 `userRateLimiter.Allow` 在 attachment validation **之前**執行（既有路徑），attachment-bearing query 仍受 RPM 限速

## 7. 文件

- [x] 7.1 `README.md::Chat UI` 加 bullet point 描述三種入口（📎 / 拖 / Cmd-V）+ 限制 + Discord attach
- [x] 7.2 `SettingsPanel.tsx::GeneralTab` 加 Chat Upload 三個 Field（Max Bytes / Max Files / Allowed MIME），使用既有 `<Field>` + `<TextInput>`
- [x] 7.3 release note 寫進本 change 的 proposal.md「不在範圍」註記；非 breaking change 不需 README 升級指引

## 8. 測試

- [x] 8.1 `tests/test-chat-upload.md` 建立，含 CU01（純文字+1 PNG）/ CU02（限制四種：超量/超大/MIME/magic mismatch）/ CU03（drag-drop）/ CU04（paste）/ CU05（Discord 收圖）/ CU06（Discord fetch fail fallback），加共通前置 + 備註
- [x] 8.1.5 對應 phase 3.5/3.6/4.7/4.8/5.6/5.7 全部由本 test 檔涵蓋
- [ ] 8.2 全套 QA cycle 跑：MT01-12（純文字回歸）、CU01-CU06（新功能）、AT-E01-04（observability 不受影響）
- [ ] 8.3 對比 archive 後的舊報告確認無 regression（`tests/test-report-2026-04-30-full-0032-round3.md`）

## 9. 結束條件

- [ ] 9.1 全套 QA cycle zero FAIL / zero env-fix-by-qa SKIP
- [x] 9.2 README、Settings UI 與 code 行為一致 — README Chat UI 段加 bullet；SettingsPanel General Tab 加 Chat Upload 三個 field；前後端對齊 chat 設定 schema
- [ ] 9.3 archive 完成
