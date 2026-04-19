## Context

Perch 現有架構：`PTYManager`（管理單一主 PTY）、`DiscordSessionManager`（channelID → PTY 的多 session 模型）、`/hook` HTTP 端點（接收 Claude/OpenCode hook 事件）、`AgentRuntime` 抽象（支援 claude / opencode）、前端為純 xterm.js terminal。

Notes2 知識庫在 `/workspace`（container 內掛載），OpenCode `as-query` agent 定義在 `.opencode/agents/as-query.md`（read-only、從 wiki 回答、mode: subagent）。

目標是讓幾十名公司使用者透過 GitLab SSO 登入後，各自獨立查詢知識庫。

## Goals / Non-Goals

**Goals:**
- GitLab OAuth2 登入，session 以 signed cookie 維持
- 每個使用者有獨立的 OpenCode PTY（`as-query` subagent 模式）
- Chat UI：使用者輸入問題 → OpenCode 輸出以 markdown 串流渲染
- 即時 tool call 狀態：PreToolUse/PostToolUse hook 事件透過 WebSocket 推送
- Admin 可用特殊 token bypass 查看所有 session（Phase 2 詳細實作，Phase 1 預留介面）

**Non-Goals:**
- 多輪對話記憶（`as-query` 每次獨立執行，不維護 conversation history）
- 檔案上傳、知識庫寫入（as-query 是 read-only）
- Admin 歷史 log 搜尋（Phase 2）
- 速率限制、使用量統計（Phase 3）

## Decisions

**1. 沿用 DiscordSessionManager 模式，新增 UserSessionManager**

`DiscordSessionManager`（channelID → `*discordSession`）已解決「多 PTY 管理 + 訂閱 + resize」的問題。`UserSessionManager` 複用同樣結構，key 改為 `userID`（從 GitLab token 取得）。實作 `SessionProvider` 介面使 server 層不需改動。

**2. GitLab OAuth2 Authorization Code Flow（server-side），與現有 auth 並存**

GitLab OAuth 由 Go 後端處理（`/auth/gitlab` → redirect → `/auth/callback` → 驗證 token → 寫 signed cookie）。前端不持有 client secret。Session cookie 使用 HMAC-SHA256 簽名，內含 `userID`、`username`、`exp`。

**並存設計**：現有 `AuthMiddleware`（none / password / mTLS）只保護 `/`、`/ws`、`/hook` 等原有路由，完全不改動。新增 `GitLabAuthMiddleware` 獨立保護 `/chat`、`/ws/chat`。兩個 middleware 在 `server.go` 的路由註冊階段分開掛載，互不干擾。`AUTH_MODE` 環境變數繼續控制原有 terminal 的認證方式。

**3. Chat UI 以獨立路由 `/chat` 提供，terminal UI 保留**

`/`（terminal）與 `/chat`（Chat UI）並列，讓現有 terminal 用戶不受影響。Chat UI 使用 React，回應內容透過 `/ws/chat?id=<userID>` WebSocket 串流，以 marked.js 渲染 markdown。

**4. Tool call 狀態透過 hook 事件推送**

OpenCode hook（PreToolUse / PostToolUse / Stop）已 POST 到 `/hook`。現有 hook handler 路由到 Discord session；擴充為同時路由到 `UserSessionManager`，依 `session_uuid` 對應到 `userID`，再透過 WebSocket 推送結構化 JSON 給前端：

```json
{ "type": "tool_start", "tool": "read", "input": {...} }
{ "type": "tool_end",   "tool": "read", "output": "..." }
```

**5. OpenCode 以 non-interactive subagent 模式啟動**

`as-query` 的 `mode: subagent` 表示接受單一 prompt 後執行完畢退出。啟動指令：

```
opencode run --agent as-query "<user question>"
```

每次查詢建立新 PTY，執行完畢後 PTY 保留輸出供訂閱，timeout 後自動清理。

## Risks / Trade-offs

- **每次查詢啟動新 PTY** → 啟動延遲約 1-2 秒。緩解：顯示 loading 狀態，使用者體驗可接受。
- **GitLab token 驗證** → 需要呼叫 GitLab API 驗證 token 有效性；離線時無法登入。緩解：session cookie 有效期內不重複驗證（TTL 8h）。
- **hook 事件與 userID 對應** → 依賴 `session_uuid`（hook payload 內）對應到 user session。需確保 `UserSessionManager` 在啟動 PTY 時記錄 session_uuid → userID 的 mapping。

## Open Questions

- GitLab instance URL 透過 `GITLAB_URL` 環境變數設定，self-hosted 與 gitlab.com 均支援。
