## Why

目前 perch 的 chat-API（web `/chat`）與 Discord adapter 只接受純文字 query。使用者要讓 Claude 看截圖、設計稿、白板照片、PDF 內頁圖時，必須先把圖描述成文字，或退回到 Claude Code 主終端機自己貼路徑。實際使用情境：

- **Web Chat**：使用者貼一張錯誤訊息截圖，期望 Claude 直接讀畫面內容回答「這個 error 怎麼修」。目前需手動打字描述截圖內容。
- **Discord**：使用者把 whiteboard 照片丟進 channel，期望 bot 把圖一起送給 Claude 分析。目前 perch 完全忽略 attachment，只送 message text。
- **行動裝置**：手機上拍照比打字快。Discord mobile + Telegram 已是「拍 → 傳」流程，只缺 perch bridge 把圖一起轉給 Claude。

Claude Code（透過 ACP `session/prompt`）已支援 multi-modal content blocks（`image` 與 `text` 混合）；perch 端只需把使用者上傳的圖檔組成正確的 ACP content block 即可。

## What Changes

- **新增** Web Chat UI 的圖片上傳：file picker + drag-drop 區塊；前端把 file 讀成 base64，附在 `POST /api/chat` body 的 `attachments` 欄位
- **新增** `POST /api/chat` 接受 `attachments: [{filename, mime_type, data_base64}]`，server 端驗證副檔名/MIME/大小後組成 ACP content block
- **新增** Discord adapter 對 inbound message 的 attachments 處理：偵測到 image attachment 時，HTTP fetch 圖檔、base64 encode、跟 message text 一起送進 ACP prompt
- **修改** `acp_process.go::PromptWithChunks` 簽名：text 參數改為 content blocks slice（或多載一個 `PromptWithContent(ctx, blocks []ACPContent, ...)`），向下相容純文字呼叫端
- **修改** `ManagementHub.SessionAdded` 與 `query_log_store.InsertSession` 的 `query` 欄位顯示：純文字 query 直接顯示；含圖時顯示成 `[image:filename] <text>` 之類的標記，避免 history 列表變成 base64 大字串
- **新增** 設定項與環境變數：
  - `CHAT_UPLOAD_MAX_BYTES`（預設 10MB）：單檔大小上限
  - `CHAT_UPLOAD_MAX_FILES`（預設 4）：單次 query 附件數量上限
  - `CHAT_UPLOAD_ALLOWED_MIME`（預設 `image/png,image/jpeg,image/gif,image/webp`）：允許的 MIME 白名單
- **新增** `tests/test-chat-upload.md`：CU01-CU06 涵蓋 file picker、drag-drop、Discord attachment、size 上限拒絕、MIME 白名單拒絕、含圖 history 列表顯示

不在本 change 範圍：
- PDF / docx 等非圖片檔（後續另開 change）
- 圖片之外的 binary（zip、video 等）
- Telegram bot 圖片支援（Telegram message 的 attachment 結構與 Discord 不同；保留給後續 change）

## Capabilities

### Modified Capabilities

- `chat-api-acp`：擴充 `POST /api/chat` 接受 `attachments`；ACP `session/prompt` content blocks 支援 image 區塊
- `discord-acp-session`：擴充 inbound message handler，把 image attachments 一併送進 ACP prompt
