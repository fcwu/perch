## Why

Notes2 知識庫目前只能由 Doro 透過 CLI 或 Perch terminal 查詢。公司團隊（幾十名工程師）需要一個可以直接使用公司 GitLab 帳號登入、提問後由 OpenCode `as-query` agent 回答的 Web UI。現有 Perch 架構（PTY、WebSocket、`/hook` 接收器）可直接擴展，無需從零開始建立。

## What Changes

- **GitLab OAuth 登入**（僅 `/chat` 路由）：公司同事以 GitLab 帳號登入後可使用知識庫查詢；原有 `/`（terminal）的 none/password/mTLS auth **不受影響，繼續運作**
- **多使用者 OpenCode session**：每個登入使用者可獨立啟動 OpenCode session（`as-query` agent），互不干擾，並行執行
- **Chat UI**：在 `/chat` 新增 Chat 模式介面，使用者輸入問題、回應以 markdown 渲染；原有 `/` terminal UI **保留不動**
- **Tool call 展開面板**：Chat UI 側邊可展開「執行細節」面板，即時顯示 OpenCode 呼叫哪些工具、目前狀態（PreToolUse / PostToolUse hook 事件驅動）

現有功能（terminal、Discord bot、Scheduler）與新功能**並存於同一 Perch process**，路由與 auth 各自獨立：

| 路由前綴 | Auth | 功能 |
|---------|------|------|
| `/`、`/ws` | 原有（none / password / mTLS）| terminal，Doro 自用 |
| `/chat`、`/ws/chat` | GitLab OAuth cookie | 知識庫查詢，公司同事 |
| `/admin/*` | ADMIN_TOKEN（Phase 2）| 管理後台 |

## Capabilities

### New Capabilities
- `gitlab-oauth`: GitLab OAuth2 PKCE 登入流程，token 驗證與 session 管理
- `user-session-manager`: 每個已認證使用者對應一個獨立 OpenCode PTY session，支援建立、查詢、終止
- `chat-ui`: Chat 介面前端，支援 markdown 渲染、串流輸出、工具狀態即時更新
- `tool-call-stream`: 解析 `/hook` 事件（PreToolUse/PostToolUse/Stop），透過 WebSocket 推送 tool call 狀態給對應使用者的前端

### Modified Capabilities
- `agent-runtime-selection`：新增 `session-target` 參數，讓 OpenCode 以 `as-query` subagent 模式啟動，而非預設的互動模式

## Impact

- `auth.go`：新增 GitLab OAuth2 middleware（`/auth/gitlab`、`/auth/callback`），**不修改**現有 password/mTLS middleware
- `im.go` / session 管理：新增 `UserSessionManager`，對應 `userID → PTY`
- `server.go`：新增 `/chat`、`/ws/chat` 端點；hook 事件路由到使用者 session
- `frontend/`：新增 Chat UI 頁面（React + xterm.js 工具面板）
- `runtime.go`：新增 `as-query` subagent 啟動模式
- 新增環境變數：`GITLAB_URL`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`
