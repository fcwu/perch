## 1. Structured Logging

- [x] 1.1 在 `main.go` 依 `LOG_FORMAT` 環境變數選擇 `slog.NewJSONHandler` 或現有 text handler
- [x] 1.2 在 `user_session.go` 的 session 建立、完成、timeout 加入標準化 slog 呼叫（`query_start`、`query_done` 含 `duration_ms`、`tool_count`）
- [x] 1.3 在 `hook.go` 的 PreToolUse handler 加入 `tool_start` log
- [x] 1.4 新增 `LOG_FORMAT` 單元測試（JSON 輸出包含必要欄位）

## 2. Per-User Rate Limiting

- [x] 2.1 新增 `ratelimit_user.go`：`UserRateLimiter` struct（`sync.Map[userID → *rate.Limiter]`）、`Allow(userID) (bool, retryAfterMs)`
- [x] 2.2 讀取 `RATE_LIMIT_RPM` 環境變數（預設 10，0 表示停用）初始化 `UserRateLimiter`
- [x] 2.3 在 `server.go` 的 `POST /api/chat` handler 前加 rate limit 檢查，超過時回 429 JSON
- [x] 2.4 新增 `ratelimit_user_test.go`：within limit、exceed limit、RPM=0 停用、兩使用者隔離

## 3. Analytics Store

- [x] 3.1 在 `store.go` 新增 `Store.GetUsageStats(from, to int64) (*UsageStats, error)`，執行 per-user aggregate + top tools query
- [x] 3.2 新增 `store_analytics_test.go`：有資料 / 無資料兩種情境

## 4. Analytics API 與 UI

- [x] 4.1 在 `server.go` 新增 `GET /admin/analytics`（需 admin cookie），呼叫 `Store.GetUsageStats` 回傳 JSON
- [x] 4.2 新增 `frontend/src/AdminAnalyticsPage.tsx`：時間範圍選擇、user stats 表格、top tools 表格
- [x] 4.3 在 `frontend/src/App.tsx` 的 admin 頁面加入 Analytics tab 路由

## 5. 文件與整合

- [x] 5.1 更新 `README.md`：新增 `RATE_LIMIT_RPM`、`LOG_FORMAT` 說明
- [x] 5.2 更新 `docs/test-cases.md`：新增 T50（rate limit 429 回應）、T51（JSON log 格式驗證）、T52（analytics API 回傳正確統計）
- [x] 5.3 執行 `go test ./...` 確認全部通過
