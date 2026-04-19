## 1. SQLite Log Store

- [x] 1.1 新增 `go.mod` 依賴：`modernc.org/sqlite`
- [x] 1.2 新增 `store.go`：SQLite 封裝，`OpenStore(path string)`、WAL mode 設定、`query_sessions` + `tool_events` schema 建立（migrate-on-open）
- [x] 1.3 實作 `Store.InsertSession`、`Store.UpdateSessionDone`、`Store.UpdateSessionTimeout`
- [x] 1.4 實作 `Store.InsertToolEvent`、`Store.UpdateToolEventEnd`
- [x] 1.5 新增 `store_test.go`：session 生命週期寫入、並行寫入不死鎖

## 2. Admin Auth

- [x] 2.1 新增 `admin_auth.go`：`POST /admin/login` handler、`ADMIN_TOKEN` 比對、寫 `perch_admin` signed cookie
- [x] 2.2 實作 admin session middleware（讀 `perch_admin` cookie → 驗簽 → 注入 context）
- [x] 2.3 `ADMIN_TOKEN` 未設定時，`/admin/*` 回傳 503
- [x] 2.4 新增 admin auth 單元測試（正確 token、錯誤 token、未設定）

## 3. Hook 與 UserSession 整合 Log Store

- [x] 3.1 `user_session.go`：session 建立時呼叫 `Store.InsertSession`；Stop hook / timeout 時呼叫 `Store.UpdateSessionDone` / `UpdateSessionTimeout`
- [x] 3.2 `hook.go`：PreToolUse 呼叫 `Store.InsertToolEvent`；PostToolUse 呼叫 `Store.UpdateToolEventEnd`
- [x] 3.3 新增整合測試：完整查詢流程後，SQLite 內有正確的 session + tool event 記錄

## 4. Admin 即時監控

- [x] 4.1 新增 `admin_realtime.go`：`AdminHub`（broadcast 差量 session 事件給所有 admin WebSocket 連線）
- [x] 4.2 `UserSessionManager` 在 session 新建 / tool 狀態變更 / session 結束時通知 `AdminHub`
- [x] 4.3 在 `server.go` 新增 `GET /ws/admin`（需 admin cookie），連線時推 snapshot，後續推差量
- [x] 4.4 新增 `frontend/src/AdminRealtimePage.tsx`：session 列表，WebSocket 差量更新

## 5. Admin 歷史搜尋

- [x] 5.1 實作 `Store.ListSessions(user, from, to, q, page, limit)`（SQLite LIKE + WHERE 組合）
- [x] 5.2 實作 `Store.GetSession(id)`（含 tool_events）
- [x] 5.3 在 `server.go` 新增 `GET /admin/history` 與 `GET /admin/history/<id>` REST 端點
- [x] 5.4 新增 `frontend/src/AdminHistoryPage.tsx`：搜尋欄、列表、session 詳情展開

## 6. Admin 路由整合與文件

- [x] 6.1 在 `frontend/src/App.tsx` 加入 `/admin`、`/admin/history` 路由；`/admin/login` 頁面
- [x] 6.2 更新 `README.md`：新增 `ADMIN_TOKEN`、`DB_PATH` 說明
- [x] 6.3 更新 `Dockerfile`：確認 `/data` volume 掛載點文件化（`VOLUME ["/data"]`）
- [x] 6.4 更新 `docs/test-cases.md`：新增 T47（admin login）、T48（即時監控）、T49（歷史搜尋）
- [x] 6.5 執行 `go test ./...` 確認全部通過
