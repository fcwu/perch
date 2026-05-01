## 0. Pre-flight

- [ ] 0.1 驗證 claude-agent-acp 2.x 對 `session/prompt` image content block 的支援：手動跑一個含 image 的 prompt，確認 RunCompleted 不報錯。記下實測 ACP 版本與 Claude model
- [x] 0.2 解 Open Questions Q1-Q4（見 design.md）— Q1 (允許 image+tool，Phase 0 實測 confirm) / Q2 (Discord 非圖 attachment 靜默 drop) / Q3 (image 不入 tool_events) / Q4 (反向不在本 change) 全部按 design.md 預設值定案

## 1. 共用：ACP content blocks 抽象

- [ ] 1.1 在 `acp_process.go` 新增：
  - `type ACPContent struct { Type string; Text string; Source *ACPImageSource }`
  - `type ACPImageSource struct { Type, MediaType, Data string }`
- [ ] 1.2 新增 `func (p *ACPProcess) PromptWithContent(ctx, blocks []ACPContent, onChunk, onToolStart, onToolEnd)`，把 blocks 直接放進 `session/prompt` 的 `prompt` 陣列
- [ ] 1.3 把舊 `PromptWithChunks(text, ...)` 改成 thin wrapper：包成單一 text block 後呼叫 `PromptWithContent`
- [ ] 1.4 unit test：`acp_process_test.go` 新增 case 驗證 PromptWithContent 把 blocks 送出的 JSON 結構正確（用 mock subprocess / golden file）

## 2. Server-side validation 共用 helper

- [ ] 2.1 新增 `attachments.go`：
  - `type Attachment struct { Filename, MimeType, DataBase64 string; size_bytes int }`
  - `ValidateAttachments(atts []Attachment, cfg AttachmentLimits) error`：跑 MIME 白名單、size、count 三檢查
  - `func MagicMime(decoded []byte) string`：讀第一 8 byte 判斷 PNG/JPEG/GIF/WEBP magic（不信任 client `mime_type`）
- [ ] 2.2 新增設定：
  - `Settings → Chat` 區段加 `upload_max_bytes` / `upload_max_files` / `upload_allowed_mime`
  - env var 對應：`CHAT_UPLOAD_MAX_BYTES`, `CHAT_UPLOAD_MAX_FILES`, `CHAT_UPLOAD_ALLOWED_MIME`
  - 預設值：10MB / 4 / `image/png,image/jpeg,image/gif,image/webp`
- [ ] 2.3 unit test：白名單拒絕 svg、size 超過拒絕、magic byte 與 client claim 不一致拒絕

## 3. Chat-API：accept attachments

- [ ] 3.1 `POST /api/chat` request struct 加 `Attachments []Attachment`
- [ ] 3.2 `chat_api_acp.go::StartSession` 簽名擴充 `attachments []Attachment`；server 端先跑 `ValidateAttachments`，失敗回 400 + JSON error
- [ ] 3.3 拼 ACP content blocks：第 0 塊是 text(query)，後續 image(att...)；呼叫 `PromptWithContent`
- [ ] 3.4 ManagementHub `SessionAdded` / `query_log_store.InsertSession` 寫入的 `query` 欄位用 placeholder 格式：`[image:foo.png] [image:bar.jpg] <原始 text>`（D4）
- [ ] 3.5 test-chat-upload.md 案例 CU01：純文字 + 1 張 PNG → ACP 收到含 image 的 prompt，回應正確
- [ ] 3.6 test-chat-upload.md 案例 CU02：上傳 4 張在限制內 OK；上傳 5 張回 400

## 4. Chat UI：upload entrypoint

- [ ] 4.1 Chat textarea 加 attach button（icon-only）開 file picker（`accept="image/*"`）
- [ ] 4.2 textarea 整塊 dropzone：dragenter 顯示 hint outline、drop 收檔
- [ ] 4.3 paste event：剪貼簿圖片自動加入；fabricate filename `pasted-<timestamp>.png`
- [ ] 4.4 顯示 chip 列：`📎 screenshot.png 240KB ✕`，按 ✕ 移除
- [ ] 4.5 送出時 `FileReader.readAsDataURL` → strip `data:` prefix → 組 `attachments` array 進 body
- [ ] 4.6 送出後清空 chip 列；error response 顯示 toast
- [ ] 4.7 test-chat-upload.md 案例 CU03：drag-drop UX 全流程
- [ ] 4.8 test-chat-upload.md 案例 CU04：paste image UX

## 5. Discord：fetch + forward attachments

- [ ] 5.1 `im_discord.go` message handler 取 `m.Attachments`，過濾 `ContentType` ∈ allow list、`Size` ≤ MAX
- [ ] 5.2 對每個過濾後 attachment：HTTP GET URL → bytes → base64 encode（`net/http` 直接拉）
- [ ] 5.3 拼 ACP content blocks 同 chat-API；呼叫 `acp_session_pool.Acquire(...)` + `PromptWithContent`
- [ ] 5.4 fetch 失敗時：log warning，繼續送 text-only prompt，最後 reply 加一行 `> 附件 <name> 下載失敗`
- [ ] 5.5 非圖片 attachment（video/audio/file）靜默 drop（D-Q2）
- [ ] 5.6 test-chat-upload.md 案例 CU05：Discord 收圖 → 送進 ACP → 回應正確
- [ ] 5.7 test-chat-upload.md 案例 CU06：Discord attachment URL fetch 失敗 → text-only fallback + 提示

## 6. 觀測 / 限速

- [ ] 6.1 attachment 數 / 總 byte 數寫進 `chat_api_acp.go` 與 `im_discord.go` 的 structured log（`slog.Info`），便於日後分析
- [ ] 6.2 `RATE_LIMIT_RPM` 既有 per-user 限速涵蓋帶 attachment 的 query；不另設 attachment-specific 限速

## 7. 文件

- [ ] 7.1 README.md 加一段「上傳圖片」說明：file picker / drag-drop / paste 三種入口；可上傳的格式與大小上限；Discord 直接 attach
- [ ] 7.2 Settings UI 對應的 tooltip 文字
- [ ] 7.3 release note：新功能（非 breaking）

## 8. 測試

- [ ] 8.1 建立 `tests/test-chat-upload.md`，內容包含 CU01-CU06（上方各 phase 對應條目）
- [ ] 8.2 全套 QA cycle 跑：MT01-12（純文字回歸）、CU01-CU06（新功能）、AT-E01-04（observability 不受影響）
- [ ] 8.3 對比 archive 後的舊報告確認無 regression（`tests/test-report-2026-04-30-full-0032-round3.md`）

## 9. 結束條件

- [ ] 9.1 全套 QA cycle zero FAIL / zero env-fix-by-qa SKIP
- [ ] 9.2 README、Settings UI 與 code 行為一致
- [ ] 9.3 archive 完成
