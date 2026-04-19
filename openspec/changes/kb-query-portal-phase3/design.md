## Context

Phase 2 後系統具備：SQLite log store、admin 即時監控、歷史搜尋。Phase 3 是「生產化」階段：讓 log 可被外部系統消費、防止濫用、提供 admin 使用分析。

Perch 目前已有 `ratelimit.go`（`/login`、`/bootstrap` 的 IP-based rate limiter，使用 `golang.org/x/time/rate`）。

## Goals / Non-Goals

**Goals:**
- `slog` JSON 格式輸出（欄位一致，方便 ELK/Loki 收集）
- Per-user sliding window rate limit（以 `RATE_LIMIT_RPM` 設定）
- Admin analytics API：per-user 查詢統計（count、avg duration）+ tool 使用次數排行
- Analytics UI：Admin 頁面新增 Analytics tab，以圖表或表格顯示

**Non-Goals:**
- 外部 metrics 系統對接（Prometheus、Grafana），留給 ops 自行設定 log shipper
- 跨天的複雜時間序列分析
- 使用者自己看自己的統計

## Decisions

**1. 沿用 `golang.org/x/time/rate` 實作 per-user limiter**

現有 `ratelimit.go` 已用此套件做 IP-based limiting。`ratelimit_user.go` 複用相同模式，key 改為 `userID`，limiter map 以 mutex 保護。

滑動窗口：`rate.NewLimiter(rate.Every(time.Minute/N), N)` 其中 N = `RATE_LIMIT_RPM`。重啟後重置（in-memory），可接受（非強一致需求）。

**2. structured-logging 以 slog JSON handler**

Go 1.21+ 內建 `log/slog`。切換 handler 為 `slog.NewJSONHandler(os.Stdout, nil)`，所有查詢事件以固定欄位記錄：

```json
{"time":"2026-04-18T10:00:00Z","level":"INFO","msg":"query_start","user_id":"123","username":"alice","session_id":"abc","query":"kubernetes probe 是什麼"}
{"time":"...","level":"INFO","msg":"tool_start","session_id":"abc","tool":"read","input_path":"wiki/concepts/kubernetes.md"}
{"time":"...","level":"INFO","msg":"query_done","session_id":"abc","duration_ms":3200,"tool_count":5}
```

**3. Analytics 以 SQLite aggregate，無需額外服務**

```sql
-- per-user stats
SELECT username, COUNT(*) as query_count,
       AVG(ended_at - started_at) as avg_duration_ms
FROM query_sessions WHERE status='done' AND started_at >= ? AND started_at <= ?
GROUP BY user_id ORDER BY query_count DESC;

-- top tools
SELECT tool_name, COUNT(*) as usage_count
FROM tool_events GROUP BY tool_name ORDER BY usage_count DESC LIMIT 10;
```

時間範圍由 admin 選擇（今天 / 本週 / 本月），前端傳 `from` / `to` Unix ms。

## Risks / Trade-offs

- **Rate limiter 重啟重置** → 極端情況下重啟可讓使用者短暫超過限制。可接受，內網使用不需強一致。
- **slog JSON 切換影響現有 log 格式** → 現有 log 已是 text 格式，切換後 log 收集設定需要調整。緩解：`LOG_FORMAT` 環境變數允許選擇 text（預設）或 json。
