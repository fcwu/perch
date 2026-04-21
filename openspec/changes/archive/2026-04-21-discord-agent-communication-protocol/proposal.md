## Why

目前 Discord integration 透過 PTY session 與 Claude Code CLI 互動——Perch 啟動一個 pseudo-terminal、直接把訊息「打字」進去，再解析終端輸出來取得回應。這個方式將 Discord session 綁定在 PTY 的文字流模型上，造成初始化暖機等待、輸出解析脆弱、permission prompt 阻塞，且 `--trust-all-tools` 控制複雜。Agent Communication Protocol（ACP）提供標準化的 JSON-RPC over stdio 介面；`claude-agent-acp`（官方 npm 套件）讓 Claude Code 原生支援 ACP 協議，Perch 可自行管理 per-channel subprocess，用結構化 JSON-RPC 取代脆弱的 PTY 橋接，同時保留 per-channel 對話上下文，不依賴任何外部 bridge service。

## What Changes

- **移除** Discord 對 PTY session 的直接依賴（不再需要 warm-up 偵測、PTY 寫入、終端輸出解析）
- **新增** ACP client，透過 HTTP 向 ACP-compatible agent server 發送 run request
- **新增** Discord session 改用 ACP runs API（`POST /runs`）發起對話，透過 SSE streaming 取得 agent 回應
- **移除** `discordSession.pty`、`warm` 狀態、PTY watcher goroutine
- **保留** Hook event 路由（`/hook` endpoint）作為 ACP 之外的補充通知機制，或逐步淘汰
- **保留** Discord 訊息分割、emoji 狀態、CJK 寬度對齊等輸出格式邏輯
- **保留** per-channel session 語義：每個 Discord channel 對應一個 `ACPProcess` subprocess，多輪對話上下文保留
- **新增** `permissionMode: "bypassPermissions"` 取代 `--trust-all-tools`，透過 ACP `new_session` 設定
- **移除** `ACP_BASE_URL`、`ACP_AGENT_NAME`（不再需要外部 bridge）
- **新增** `DISCORD_ACP_ENABLED` 環境變數取代 `ACP_BASE_URL` 作為 ACP 模式開關

## Capabilities

### New Capabilities
- `acp-client`: ACP HTTP client，封裝 runs API 請求、SSE streaming 解析、run 生命週期管理（create → stream → done/error）
- `discord-acp-session`: Discord session 改用 ACP client 取代 PTY，處理 message → ACP run → Discord reply 完整流程

### Modified Capabilities
- `discord-open-channels`: Discord 公開頻道處理邏輯不變，但 session 底層從 PTY 換成 ACP run；移除 warm-up 等待行為
- `discord-dm-allowlist`: DM allowlist 驗證邏輯不變，session 底層同上改為 ACP run

## Impact

- `im_discord.go`：主要改寫對象，移除 PTY 相關欄位與邏輯，改呼叫 ACP client
- `im.go`：`IMAdapter.Start(*PTYManager)` 介面可能需調整（Discord 不再需要 PTYManager）
- `hook.go`：`/hook` endpoint 與 hook routing 可能簡化或移除（視 ACP 是否提供等效事件）
- `runtime.go`：ACP server URL 設定需新增到 runtime 或環境變數
- 新增 `acp_client.go`：封裝 ACP protocol 邏輯
- 環境變數新增：`ACP_BASE_URL`（agent ACP server 位址）
