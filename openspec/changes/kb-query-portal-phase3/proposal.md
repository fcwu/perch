## Why

Phase 1+2 提供了查詢功能與管理員監控，但缺少兩個生產環境必要的機制：（1）防止單一使用者佔用過多資源（rate limiting）；（2）讓管理員理解整體使用趨勢（usage analytics）。此外，log 的格式與完整度需要進一步精緻化，以便對接公司既有的 log 收集系統（如 ELK / Loki）。

## What Changes

- **結構化 JSON log**：所有查詢相關事件以 `slog` JSON 格式輸出，欄位標準化（`user_id`、`session_id`、`tool`、`duration_ms` 等），方便 log aggregator 收集
- **Per-user rate limiting**：每個使用者每分鐘最多 N 次查詢（`RATE_LIMIT_RPM` 設定），超過時回傳 429 並顯示剩餘等待時間
- **使用量統計 API**：Admin 可查詢各使用者的查詢次數、平均 duration、最常使用的 tool；以 SQLite aggregate query 計算，無需額外服務

## Capabilities

### New Capabilities
- `structured-logging`: 標準化 JSON log 輸出，涵蓋所有查詢生命週期事件
- `user-rate-limit`: per-user sliding window rate limiting（in-memory，重啟後重置）
- `usage-analytics`: Admin 使用量統計 API + UI（per-user 查詢數、平均 duration、tool 使用分佈）

### Modified Capabilities
- `query-log-store`：新增 `analytics` SQL view，供 usage-analytics 使用

## Impact

- `logger.go` / `main.go`：切換為 JSON slog handler，新增查詢相關 log 欄位
- 新增 `ratelimit_user.go`：per-user sliding window rate limiter（複用現有 `ratelimit.go` 的框架）
- `server.go`：在 `/api/chat` 前加 rate limit middleware
- `store.go`：新增 analytics view + `Store.GetUsageStats(from, to)` 方法
- 新增 `GET /admin/analytics` 端點與 `frontend/src/AdminAnalyticsPage.tsx`
- 新增環境變數：`RATE_LIMIT_RPM`（預設 10）
