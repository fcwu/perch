## 背景

Perch 透過 SSE 將 Agent（Claude Code ACP）回應傳給 Web chat，透過 Bot 訊息傳給 Discord。Agent 文字輸出以 `RunCompleted` 事件攜帶完整文字主體。目前沒有機制讓 Agent 輸出圖片——Playwright 截圖、圖表、QR code 全部遺失。使用者已有 Playwright MCP，希望在對話中截圖後能在 Web chat 與 Discord 看到圖片。

## 目標 / 非目標

**目標：**
- Agent 在回應文字中加入 `[image: <路徑>]` 即可輸出圖片
- Web chat 在助理訊息氣泡內嵌顯示圖片
- Discord 在同一則回覆中以檔案附件傳送圖片
- 圖片隨 ACP session 清理自動刪除

**非目標：**
- 串流中途傳送圖片（僅支援 `RunCompleted` 後處理）
- Agent 無文字說明的靜默圖片推送
- AI 圖片生成
- 使用者 → Agent 的附件上傳方向（已由現有 upload 系統處理）

## 決策

### D1：內嵌語法，不新增 ACP 事件型別

**選擇**：Agent 在文字中寫 `[image: /絕對路徑.png]` 或 `[image: data:image/png;base64,...]`，Perch 在 `RunCompleted` 後處理時提取這些語法。

**原因**：不需變更 ACP 協定，不需修改 Claude Code SDK，只要在 system prompt 加一行說明即可。若改用新 ACP 事件型別，需要修改 ACP client library 並處理版本協商。

**替代方案**：讓 Agent 明確呼叫 MCP tool `send_image(path)`。捨棄原因：需在每個 workspace 暴露新 MCP tool，多一個 round-trip，且 tool result 的路由比回應後處理複雜。

### D2：Server 儲存圖片，透過 `/api/images/<id>` 提供存取

**選擇**：`RunCompleted` 時，Perch 提取圖片語法、將每張圖片複製到 `<workdir>/images/<conv-id>/<uuid>.<ext>`，並透過 `GET /api/images/<conv-id>/<uuid>.<ext>`（需 session cookie 驗證）提供存取。

**原因**：避免在 SSE 事件中嵌入大型 base64 blob。前端只需取得 URL 再 render `<img src=...>`；Discord 也可下載後再上傳。

**替代方案**：在 SSE/WebSocket 訊息中直接夾帶 base64。捨棄原因：1 MB 截圖換算 base64 約 1.3 MB JSON，會撐大 log 與 SSE buffer。

### D3：Discord 使用 DiscordGo `File` 欄位傳送檔案附件

**選擇**：`RunCompleted` 後，對每張圖片讀取檔案並加入 `MessageSend.Files` slice，與文字 chunk 在同一個 `ChannelMessageSendComplex` 呼叫中送出。

**原因**：Discord 原生圖片預覽，不需外部 hosting。檔案 ≤ 8 MB 在 Discord 免費方案限制內。

**替代方案**：在文字中貼 `/api/images` URL。捨棄原因：Discord 不會對需要驗證的 URL 產生預覽縮圖，且 URL 為內部位址。

### D4：清理機制與 upload orphan TTL 對齊

`<workdir>/images/<conv-id>/` 在 ACP session pool evict `(user, conv)` 時刪除；Perch 啟動時掃描超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS`（預設 7 天）的目錄並刪除。不需新增設定項。

## 風險 / 取捨

- **路徑穿越攻擊** → 驗證解析後的絕對路徑必須在 `/tmp` 或 `<workdir>` 之下；否則記錄安全警告並略過
- **大型圖片卡住 Discord** → Discord 檔案上傳是同步操作，10 MB PNG 可能延遲回覆。緩解：上限 8 MB（Discord 限制），超過則略過並在文字加上說明
- **Agent 幻覺路徑** → 若檔案不存在，記錄警告、從文字中刪除語法 token，其餘訊息不受影響
- **base64 內嵌支援** → 接受為次要格式（Playwright 可直接回傳 base64），解碼後存檔再提供存取。最大允許 base64 解碼後 8 MB

## 部署計畫

1. 部署後端變更（新增 `/api/images` 路由、`RunCompleted` 後處理器、image store）
2. 部署前端變更（訊息氣泡渲染圖片）
3. 無資料庫 migration；圖片純檔案系統儲存
4. 回滾：移除後處理器，`[image: ...]` 語法以純文字顯示（不影響功能）

## 待解問題

- Q1：`/api/images` 應使用 session cookie 驗證還是簽署 URL？（目前計畫：session cookie，與其他 `/api/*` 端點一致）
- Q2：`[image: ...]` 語法在顯示文字中是否完全刪除，或保留為圖片說明？（目前計畫：刪除 token，以檔名作為圖片下方 caption）
