## 背景

Perch 的網頁聊天（`/chat`）目前將每次查詢作為獨立的一次性 agent session 執行。`UserSessionManager.StartSession` 每次都啟動全新的 agent 程序；前端在每次送出時都會重置顯示。

`query_sessions` SQLite 資料表已記錄每一次查詢及其完整回應，以 `user_id` 為索引，並帶有 `started_at` 時間戳記與 `status`。管理員歷史頁面（`/admin/history`）已從此資料表讀取資料。這代表對話歷史早已被持久化——只是沒有被用來建構上下文。

## 目標 / 非目標

**目標：**
- 使用者可以在過去 24 小時內追問後續問題，引用先前的輪次。
- 伺服器從 `query_sessions` 重建歷史——不需要新資料表，不需要記憶體 map。
- 對話在 24 小時無活動後自動過期。
- 前端在可捲動的對話串中渲染所有輪次。
- 「新對話」讓使用者可以立即重置，無需等待過期。
- 現有的呼叫方（Discord、Telegram、排程器）完全不受影響。

**非目標：**
- 超過 24 小時的對話。
- Discord / Telegram / 排程器流程中的對話歷史。
- 對先前輪次進行串流部分更新。
- 多裝置或共享對話 session。

## 決策

### D1 — 透過 prompt 前綴注入歷史，而非長時間運行的程序

**決策：** 在啟動每個新 PTY session 之前，將序列化的對話歷史前置於查詢字串。

**理由：** PTY-based agent runtime 本質上是短暫存在的。在輪次之間保持持久 agent 程序會引入超時複雜性、記憶體洩漏以及 session 恢復問題。從 agent 的角度來看，prompt 注入是無狀態的，且與所有 runtime 相容。

**格式：**
```
<conversation_history>
User: <第 1 輪查詢>
Assistant: <第 1 輪回應>
User: <第 2 輪查詢>
Assistant: <第 2 輪回應>
</conversation_history>

<當前查詢>
```

### D2 — 歷史來源為現有的 query_sessions 資料表

**決策：** 透過查詢 `query_sessions WHERE user_id=? AND status='done' AND started_at >= now-24h ORDER BY started_at ASC LIMIT 20` 重建對話歷史。

**理由：** 資料早已存在——每一次完成的網頁聊天查詢都以 `query` 和 `response` 欄位儲存。引入獨立的記憶體 map 會重複這些資料，且在重啟後遺失歷史。使用 SQLite 可免費獲得持久性，並讓歷史在管理面板中保持可見。

**考慮過的替代方案：** 記憶體中的 `map[userID][]ConversationTurn`。已拒絕，原因：重複了 SQLite 中已有的資料、重啟後遺失歷史，且需要獨立的驅逐機制。

### D3 — 以 24 小時滑動視窗作為 TTL

**決策：** 歷史查詢使用 `started_at >= now - 24h`。超過 24 小時的 session 不包含在前綴中；它們仍保留在 SQLite 以供管理員歷史使用。

**理由：** 簡單，不需要新欄位。符合「擱置超過一天的對話應從頭開始」的預期。24 小時視窗定義為具名常數，可在不變更結構的情況下調整。

### D4 — new_conversation 旗標跳過歷史查詢

**決策：** `POST /api/chat` 接受 `new_conversation: bool`。當為 `true` 時，`StartSession` 完全跳過 `GetRecentHistory` 呼叫，並將空歷史傳入 `buildPrompt`。

**理由：** 提供明確的使用者控制重置機制，無需刪除資料列或新增重置時間戳記欄位。在單一請求中同時清除 UI（前端）+ 跳過歷史旗標（後端），從使用者角度來看，重置是原子操作。

### D5 — 對話識別以已認證的使用者 ID 為鍵

**決策：** 歷史以 GitLab auth context 中的 `userID` 為鍵。客戶端不需要傳送任何額外識別符。

**理由：** 所有網頁聊天使用者都必須登入（需要 GitLab OAuth）。使用者 ID 在每個請求中都已存在——不需要客戶端生成 UUID。一個已認證的使用者 = 一個對話執行緒，符合現有的單一 active session 限制。

## 風險 / 取捨

- **長回應導致 prompt 膨脹**：完整的 agent 輸出可能很大；20 輪上限 + 24 小時視窗限制最壞情況。未來改進方向：摘要舊輪次或截斷回應片段。
- **管理員可見歷史**：現有行為——回應已儲存在 `query_sessions` 中，無新暴露。
- **多標籤頁並發**：同一使用者的兩個標籤頁共享以 userID 為鍵的歷史。可接受的限制——單一 active session 限制已序列化 agent 調用。

## 遷移計畫

1. 部署不破壞現有行為：`new_conversation` 為可選；缺省 = false = 現有行為延伸加上歷史。
2. 不變更結構——現有 `query_sessions` 資料列為唯讀。
3. 回滾：還原二進位；`query_sessions` 資料表不受影響。

## 開放問題

- 建構 prompt 時，assistant 回應應存儲原文還是截斷至每輪最大長度？→ 預設全文（去除 ANSI）；若 prompt 大小成為問題，再加入每輪截斷。
