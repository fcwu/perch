## 為什麼

目前透過網頁聊天介面送出的每一個問題都會啟動一個全新的 agent session，完全沒有記憶任何先前的對話。使用者無法追問後續問題、引用先前的回答，或在多輪對話之間保留上下文——每則訊息都是孤立存在的。

## 變更內容

- 後端從現有的 `query_sessions` SQLite 資料表重建對話歷史——不需要新增任何儲存空間。每次查詢時，會擷取該使用者過去 24 小時內的 `done` session（依時間正序排列），並將其附加到新查詢的前方，再送給 agent 執行。
- 對話自動過期：若使用者超過 24 小時沒有任何活動，下一次查詢將以空白歷史重新開始。
- `/api/chat` endpoint 接受一個可選的布林欄位 `new_conversation`。當設為 `true` 時，伺服器跳過歷史查詢，直接以全新狀態啟動。
- 前端將所有輪次累積在一個可捲動的對話串（alternating user / assistant 氣泡），而不是只顯示最近一次的交換。
- 「新對話」按鈕讓使用者可以立即重置，無需等待過期。
- 歷史記錄已透過現有的 `query_sessions` 資料表持久化——管理員歷史頁面 `/admin/history` 仍會顯示所有 session，不受影響。

## 能力

### 新增能力

- `multi-turn-chat`：網頁聊天介面的完整多輪對話支援——歷史從 SQLite 重建，前端以對話串方式渲染。

### 修改能力

<!-- 此提案未修改任何現有的規格層級需求。 -->

## 影響範圍

- **`store.go`**：新增 `GetRecentHistory(userID string, since, limit int64)`——查詢 `query_sessions` 中最近完成的 session。
- **`user_session.go`**：`StartSession`——呼叫 `GetRecentHistory` 並將結果傳入 `buildPrompt`；接受 `newConversation bool` 以跳過歷史。
- **`server.go`**：`handleChatAPI`——讀取 `new_conversation` 旗標並傳入 `StartSession`。
- **`frontend/src/ChatPage.tsx`**：將單輪顯示改為訊息陣列；新增「新對話」控制項。
- **不變更資料庫結構**——重用現有的 `query_sessions` 資料表。
- **不破壞 API 相容性**——`new_conversation` 為可選（預設 false）；Discord、Telegram 與排程器呼叫方不受影響。
