## 為什麼

Agent 使用瀏覽器自動化或其他工具產生圖片（截圖、圖表、QR code）時，這些圖片目前會遺失——Agent 只能描述所見，無法將視覺內容回傳給使用者。支援 Agent 主動輸出圖片，可補齊 Web Chat 與 Discord 這兩個管道的缺口。

## 變更內容

- Agent 回應可在文字中嵌入 `[image: <路徑>]` 語法來附帶一張或多張圖片
- Web chat 前端在訊息氣泡內嵌顯示圖片
- Discord 整合在同一則訊息以檔案附件方式傳送圖片
- Server 端解析圖片語法、暫存圖片並提供 HTTP 端點供前端存取
- 清理機制與現有 upload orphan TTL 對齊，不需額外設定

## 能力範圍

### 新增能力

- `agent-image-output`：Agent 回應可攜帶圖片；Server 儲存並提供存取端點；Web Chat 與 Discord 分別渲染或附帶圖片

### 修改能力

- `chat-ui`：訊息氣泡需渲染 Agent 回應中的內嵌圖片
- `discord-acp-session`：需將圖片以 Discord 檔案附件方式轉發
- `acp-tool-events`：`RunCompleted` 處理流程新增圖片提取步驟

## 影響範圍

- **後端**：新增圖片儲存端點、ACP session handler 解析圖片語法、清理機制與 upload orphan TTL 對齊
- **前端**：`MessageBubble` 元件、`useChat` hook 可能需調整
- **Discord**：`discord.go` 訊息發送路徑
- **ACP 協定**：無協定層變更，僅在 `RunCompleted` 文字後處理階段新增解析邏輯
