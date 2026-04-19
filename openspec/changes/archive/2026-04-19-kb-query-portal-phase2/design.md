## Context

Phase 1 完成後，`UserSessionManager` 管理 userID → PTY session，hook 事件路由到 `/ws/chat`。目前沒有任何持久化：session 結束後資料消失，管理員也沒有監控介面。

Phase 2 在此基礎上加入：SQLite 持久化 → admin auth → 即時監控 → 歷史搜尋。

## Goals / Non-Goals

**Goals:**
- SQLite 記錄每次查詢的完整生命週期（query text、tool call sequence、final response、duration）
- Admin 以 `ADMIN_TOKEN` 登入，不走 GitLab OAuth
- Admin 即時頁面：WebSocket 推送 active sessions 清單及每個 session 的 tool 狀態
- Admin 歷史頁面：列表 + 搜尋（user、時間範圍、query keyword）+ 展開看單一 session 詳情

**Non-Goals:**
- 使用者自己查看自己的歷史（Phase 3 可選擴充）
- 匯出 CSV / 報表（Phase 3）
- 多 admin 帳號（單一 `ADMIN_TOKEN` 足夠）

## Decisions

**1. SQLite 作為 log store**

理由：單機部署、資料量小（幾十人、每人每天幾十次查詢）、無需額外服務、Go 有 `modernc.org/sqlite`（pure Go，不需 CGO）。

Schema：
```sql
CREATE TABLE query_sessions (
  id          TEXT PRIMARY KEY,   -- session_uuid
  user_id     TEXT NOT NULL,
  username    TEXT NOT NULL,
  query       TEXT NOT NULL,
  response    TEXT,               -- 填入 Stop 事件後
  started_at  INTEGER NOT NULL,   -- Unix ms
  ended_at    INTEGER,
  status      TEXT NOT NULL       -- "running" | "done" | "timeout" | "error"
);

CREATE TABLE tool_events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id   TEXT NOT NULL REFERENCES query_sessions(id),
  tool_name    TEXT NOT NULL,
  input_json   TEXT,
  output_json  TEXT,
  started_at   INTEGER NOT NULL,
  ended_at     INTEGER
);
```

**2. Admin auth 獨立於 GitLab OAuth**

Admin 以 `POST /admin/login` 送 `{"token": "..."}` 比對 `ADMIN_TOKEN`，成功後寫 `perch_admin` signed cookie（TTL 24h）。Admin middleware 與 user middleware 完全分開，互不影響。

**3. Admin 即時監控以 WebSocket 推送差量更新**

`/ws/admin` 連線後，server 先推送所有 active sessions 快照，後續每當任何 session 有狀態變更（新建、tool start/end、完成）時推送 patch 事件：

```json
{ "type": "session_update", "session": { "id": "...", "username": "...", "status": "running", "current_tool": "read" } }
{ "type": "session_removed", "id": "..." }
```

理由：避免全量輪詢，前端維護一份 session map，差量更新渲染。

**4. 歷史搜尋以 REST API**

`GET /admin/history?user=&from=&to=&q=&page=&limit=` 回傳 JSON，前端分頁顯示。單一 session 詳情：`GET /admin/history/<session_id>`，包含 tool_events 序列。

## Risks / Trade-offs

- **SQLite 寫入並行** → 多個 session 並行寫入；`modernc.org/sqlite` 支援 WAL mode，並行讀不影響寫，風險低。
- **response 欄位大小** → OpenCode 回答可能很長；SQLite text 無上限，但查詢時傳輸量大。緩解：list API 不回傳 response，只在詳情 API 回傳。
- **Admin token 洩漏** → 單一 token，需靠環境變數保護；如洩漏只能重啟服務換 token。可接受的風險範圍（內網部署）。
