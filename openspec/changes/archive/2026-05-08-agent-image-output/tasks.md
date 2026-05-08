## 1. 圖片儲存模組（後端）

- [x] 1.1 建立 `imagestore` package：`Store(convID, filename string, r io.Reader) (string, error)` 寫入 `<workdir>/images/<conv-id>/<uuid>.<ext>` 並回傳相對 URL 路徑
- [x] 1.2 加入路徑安全驗證：拒絕解析後超出 `/tmp` 或 `<workdir>` 範圍的路徑；拒絕超過 8 MB 的檔案
- [x] 1.3 加入 `Cleanup(convID string)`：刪除 `<workdir>/images/<conv-id>/` 目錄
- [x] 1.4 在 `main.go` 加入啟動孤兒掃描：掃描 `<workdir>/images/*/` 並刪除 mtime 超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS` 的目錄

## 2. 圖片提取——ACP RunCompleted 後處理器

- [x] 2.1 實作 `ExtractImages(text string, store *imagestore.Store, convID string) (cleanText string, attachments []ImageAttachment, err error)`，使用 regex 找出 `[image: <路徑或data-uri>]` token
- [x] 2.2 處理檔案路徑情境：開啟檔案、呼叫 `store.Store`、append `ImageAttachment{URL, Caption}`
- [x] 2.3 處理 `data:image/*;base64,...` 情境：解碼 base64、呼叫 `store.Store`、append 附件
- [x] 2.4 處理例外：檔案不存在記錄警告並刪除 token；檔案過大則在文字加上 `(圖片過大，無法顯示)` 說明
- [x] 2.5 將 `ExtractImages` 接入 Chat API ACP 路徑的 `RunCompleted` handler
- [x] 2.6 將 `ExtractImages` 接入 Discord ACP 路徑的 `RunCompleted` handler

## 3. 圖片 HTTP 端點

- [x] 3.1 在 router 註冊 `GET /api/images/{convID}/{filename}` 路由
- [x] 3.2 套用與其他 `/api/*` handler 相同的 session cookie 驗證 middleware
- [x] 3.3 以正確 `Content-Type`（依副檔名推斷）與 `Cache-Control: private, max-age=86400` 回傳檔案
- [x] 3.4 檔案不存在回傳 404；未驗證請求回傳 401

## 4. ACP Session Pool——清理掛鉤

- [x] 4.1 在 `acp_session_pool.go` 淘汰 callback 中，於 session 拆除後呼叫 `imagestore.Cleanup(convID)`

## 5. SSE——攜帶圖片附件至 Web Chat

- [x] 5.1 在共用型別中定義 `ImageAttachment` struct：`{URL string, Caption string}`
- [x] 5.2 在 SSE `message` 事件 JSON 酬載中加入 `Images []ImageAttachment` 欄位
- [x] 5.3 確認 `Images` 為空或 nil 時既有 SSE 事件處理不受影響

## 6. Discord——檔案附件

- [x] 6.1 `ExtractImages` 完成後，對每個附件讀取已儲存檔案並加入 `MessageSend.Files`（`discordgo.File{Name, Reader}`）
- [x] 6.2 略過超過 8 MB 的檔案，並在最後一個文字 chunk 附加 `(圖片過大，無法傳送至 Discord)`
- [x] 6.3 檔案附件發送包在錯誤處理中：記錄失敗、繼續傳送文字回覆

## 7. 前端——內嵌圖片渲染

- [x] 7.1 更新 `ChatMessage` / `MessageBubble` 元件以讀取訊息酬載中的 `images` 欄位
- [x] 7.2 在 markdown 文字下方渲染每張圖片：`<img src={url} alt={caption} />`
- [x] 7.3 為 `<img>` 加入 `onerror` handler，顯示破圖佔位符並以 caption 作為備用說明
- [x] 7.4 建置並手動測試：傳送含 `[image: ...]` 語法的訊息，確認圖片正確內嵌顯示

## 8. System Prompt 說明

- [x] 8.1 在預設 system prompt（或 workspace bootstrap）加入一行說明：`若要傳送圖片給使用者，請在回應文字中加入 [image: /路徑/檔案.png]`
