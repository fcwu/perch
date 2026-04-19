## Why

Phase 1 建立了多使用者查詢入口，但管理員無法看到「誰在問什麼」、「現在有哪些 session 在跑」、「過去的查詢紀錄是什麼」。對公司內網部署而言，可觀測性（observability）與稽核（audit）是基本需求：管理員需要即時掌握系統狀態，並能事後追溯任何一次查詢的完整過程。

## What Changes

- **結構化 log 持久化**：每次查詢的完整記錄（userID、username、query、tool calls、response、duration）寫入 SQLite DB
- **Admin 即時監控頁面**：特殊 admin token 登入後，可看到所有目前進行中的 session、每個 session 的使用者與目前執行中的 tool
- **歷史查詢搜尋**：Admin 可依使用者、時間範圍、關鍵字搜尋過去的查詢紀錄，查看完整的問題與回答
- **Admin auth**：獨立於 GitLab OAuth，以 `ADMIN_TOKEN` 環境變數設定，透過 `/admin/login` 取得 admin session cookie

## Capabilities

### New Capabilities
- `query-log-store`: 查詢紀錄持久化（SQLite），含 session 生命週期事件寫入
- `admin-auth`: Admin token 登入，獨立 session，與 GitLab OAuth 使用者 session 分開管理
- `admin-realtime`: 即時監控頁面，WebSocket 推送所有 active session 狀態更新
- `admin-history`: 歷史查詢搜尋 API + UI，支援依 user / 時間 / 關鍵字過濾

### Modified Capabilities
- `tool-call-stream`：hook 事件除了推給前端 WebSocket，同時寫入 `query-log-store`
- `user-session-manager`：session 建立與結束時觸發 log 寫入，並通知 `admin-realtime` 更新

## Impact

- 新增 `store.go`：SQLite 封裝（`query_sessions` + `tool_events` 兩張表）
- 新增 `admin.go`：admin auth middleware + `/admin/login`、`/admin/sessions`、`/admin/history`、`/ws/admin` 端點
- `hook.go`：hook 事件同時寫入 log store
- `user_session.go`：session 生命週期事件寫入 log store
- 新增 `frontend/src/AdminPage.tsx`：即時監控 + 歷史搜尋 UI
- 新增環境變數：`ADMIN_TOKEN`、`DB_PATH`（SQLite 檔案路徑，預設 `/data/perch.db`）
