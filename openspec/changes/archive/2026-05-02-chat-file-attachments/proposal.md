## Why

Chat 目前只能附 image（PNG/JPEG/GIF/WebP），靠 ACP `image` content block 內嵌進 LLM context。實際情境裡使用者需要附的不只是圖：

- **文字類檔**：log 檔、CSV 報表、JSON dump、Markdown 草稿、原始碼片段。這些貼進輸入框會把對話塞爆，且失去檔案脈絡。
- **PDF**：合約、規格書、論文。LLM 多模態原生看不懂 PDF，但 agent 端的 Read/Bash 工具可處理（pdftotext 已是常見能力）。
- **影片 / 音訊**：會議錄音要轉錄、bug 重現錄影要 metadata、語音留言要逐字稿。完全不可能 inline 進 context。
- **其他二進位**：zip、binary log 等需要 agent 自行決定怎麼分析的檔。

關鍵觀察：perch 的 agent 都是有 tool-use 能力的（claude-code、codex、opencode、sst），它們已具備 Read/Bash 工具，能自主呼叫 `file`、`pdftotext`、`ffprobe`、`whisper`、`jq` 等去分析任何檔案 —— 只要檔案落在 agent workdir 內，並在 prompt 裡告訴它路徑。這比「想辦法把所有格式塞進 LLM context」務實太多。

## What Changes

- **新增** 「落盤附件」傳輸路徑：非 image 類附件由 server 寫入 `<workdir>/uploads/<conv-id>/<filename>`，並在 prompt 文字前綴注入引用 `[file: ./uploads/<conv-id>/<filename> (mime, size)]`，由 agent 用既有工具自主讀取分析
- **保留** image 走原本 ACP `image` content block 內嵌路徑（fast vision，不落盤），舊行為 100% 不變
- **修改** `POST /api/chat` 的 `attachments` 處理：server 依 MIME 分流（image → ACP block，其他 → 落盤 + prompt 前綴），單一 request 可混合兩種類型
- **修改** `attachments.go`：`MagicMime` 擴充支援文字類（text/plain、text/markdown、text/csv、application/json、text/x-log）+ application/pdf；新增 `WriteAttachmentsToDisk` 與 `BuildPromptFilePrefix` 函式
- **修改** Discord adapter (`im_discord.go`)：對非 image attachment 走相同落盤路徑（HTTP fetch → 寫入 conv-scoped 目錄）
- **修改** `ChatSettings` / 環境變數：
  - `CHAT_UPLOAD_MAX_BYTES` 預設值維持 10 MB（足夠涵蓋多數文字類 / 短音訊），但 admin 可調高
  - 新增 `CHAT_UPLOAD_DIR_QUOTA_BYTES`（預設 500 MB）：每個 conversation uploads 目錄總容量上限
  - 新增 `CHAT_UPLOAD_ALLOWED_MIME` 預設值擴充：text/plain, text/markdown, text/csv, application/json, application/pdf, application/x-ndjson（image 系列保留）
- **新增** uploads 目錄清理：conversation 終結（pool 退出 / 手動刪除 history）時刪除對應 `<workdir>/uploads/<conv-id>/`；perch 啟動時掃描遺留目錄並清掉
- **新增** Frontend `ChatPage.tsx`：`accept` 屬性放寬到 allow-list 全部、attachment chip 顯示非圖檔的 icon + filename + size、上傳 progress 對大檔顯示進度條
- **新增** `tests/test-chat-file-attach.md`：CFA01-CFA08 涵蓋 text/PDF 落盤、混合 image+file、超過單檔上限拒絕、超過目錄總額拒絕、會話結束自動清理、Discord 非圖檔附件

### Phase 拆分

本 change 一次涵蓋 **Phase 1**（基礎設施 + 文字類檔 + PDF）。**Phase 2**（影片 / 音訊 + runtime image 加 ffprobe / whisper）獨立成另一個 change，因為涉及 runtime image 體積與工具選型（whisper-cli vs whisper.cpp vs 外部 API），需要獨立驗證。

### 不在本 change 範圍

- 影片 / 音訊 MIME（Phase 2）
- runtime image 安裝額外分析工具（Phase 2）
- 直接把檔案內容塞進 LLM context 的優化（agent 自己 Read 就夠用）
- 多人共享同一份上傳檔（每個 conversation 自己一份）
- Telegram 非圖附件（與 Discord 結構不同，獨立 change）

## Capabilities

### Modified Capabilities

- `chat-api-acp`：`/api/chat` 的 `attachments` 處理新增 disk-save 分支；非 image 附件以 prompt 文字前綴 `[file: ...]` 引用，不進 ACP image block
- `discord-acp-session`：Discord adapter 對非 image attachment 走 disk-save，與 `/api/chat` 等價（取代既有「非圖檔靜默丟棄」行為）

> Note：前端 UI 變更（accept 放寬、attachment chip 樣式）屬實作細節，不額外增刪 `chat-ui` 的 spec requirement，僅列在 tasks 中。

## Impact

- **Code**：`attachments.go`（新增 disk write、prompt prefix 函式）、`chat_api_acp.go`（attachment 分流）、`im_discord.go`（非圖檔處理）、`bootstrap.go`（啟動清理遺留 uploads/）、`user_session.go` 或 `chat_api_acp.go`（session 退出時刪 conv 目錄）、`frontend/src/ChatPage.tsx`（accept + chip UI）、`server.go`（如需新增 settings 欄位）
- **APIs**：`POST /api/chat` 的 `attachments` 物件 wire shape 不變（仍是 `{filename, mime_type, data_base64}`），但允許的 MIME 集合擴大；prompt 文字會被 server 自動加入 `[file: ...]` 前綴
- **Filesystem**：agent workdir 下新增 `uploads/<conv-id>/` 子樹；單會話容量上限預設 500 MB；perch 啟動掃描遺留目錄
- **Settings**：`CHAT_UPLOAD_ALLOWED_MIME` 預設值擴大；新增 `CHAT_UPLOAD_DIR_QUOTA_BYTES`
- **Backwards compatibility**：image 路徑與 wire shape 完全不變；舊 client 只送 image 仍 100% 正常工作；不送 attachments 的純文字 query 完全沒影響
- **依賴**：本 change 不引入新的 runtime 依賴（agent 自帶 Read/Bash 已足夠處理文字 + PDF；PDF agent 端可呼叫既有 pdftotext/Read 解析）
