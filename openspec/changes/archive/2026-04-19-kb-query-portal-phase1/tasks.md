## 1. GitLab OAuth

- [x] 1.1 新增 `gitlab_auth.go`：實作 `/auth/gitlab`（redirect）和 `/auth/callback`（code exchange、fetch user profile、寫 signed cookie）
- [x] 1.2 新增 `session_cookie.go`：HMAC-SHA256 signed cookie 的 sign / verify / parse 工具函式
- [x] 1.3 在 `server.go` 新增 `/auth/gitlab`、`/auth/callback` 路由，並加入 chat session auth middleware（無效 cookie → redirect `/auth/gitlab`）
- [x] 1.4 新增 GitLab OAuth 單元測試（callback state mismatch、token exchange error、valid flow mock）

## 2. UserSessionManager

- [x] 2.1 新增 `user_session.go`：`userSession` struct（PTY + 訂閱 channel + session_uuid mapping）與 `UserSessionManager`（userID → session map、mutex）
- [x] 2.2 實作 `UserSessionManager.StartSession(userID, username, query string) error`：啟動 OpenCode PTY（`opencode run --agent as-query "<query>"`）並記錄 session_uuid
- [x] 2.3 實作 `UserSessionManager` 的 `SessionProvider` 介面（`SubscribeSession`、`ResizeSession`、`WriteSession`、`ListSessions`）
- [x] 2.4 實作 session timeout（5 分鐘）與 PTY EOF 偵測 → session 標記 completed，5 分鐘後清理
- [x] 2.5 新增 `UserSessionManager` 單元測試（session 建立、已有 session 返回 409、timeout 清理）

## 3. AgentRuntime 擴充

- [x] 3.1 在 `runtime.go` 的 `AgentRuntime` 新增 `RunAgent(agentName, prompt, workdir string) (string, []string)` 方法，回傳 opencode subagent 啟動命令與參數
- [x] 3.2 新增 `RunAgent` 單元測試

## 4. Hook 事件路由

- [x] 4.1 擴充 `hook.go` 的 hook handler：收到 hook 事件時，依 `session_uuid` 查找 `UserSessionManager`，找到後發送 `tool_start` / `tool_end` / `done` JSON 到 WebSocket 訂閱者
- [x] 4.2 在 `server.go` 新增 `/api/chat`（POST，接收 query，呼叫 `UserSessionManager.StartSession`）與 `/ws/chat`（WebSocket，訂閱 user session output）
- [x] 4.3 新增 hook 路由單元測試（已知 session_uuid → 正確路由；未知 → 丟棄）

## 5. Chat UI 前端

- [x] 5.1 新增 `frontend/src/ChatPage.tsx`：query 輸入欄、markdown 串流輸出區、loading 狀態
- [x] 5.2 新增 `frontend/src/ToolPanel.tsx`：可收合的 tool call 面板，接收 `tool_start` / `tool_end` JSON 更新狀態
- [x] 5.3 在 `frontend/src/App.tsx` 加入 `/chat` 路由（React Router），auth redirect 到 `/auth/gitlab`
- [x] 5.4 新增 marked.js 依賴，Chat 輸出以 markdown 渲染

## 6. 整合測試與文件

- [x] 6.1 更新 `README.md`：新增 `GITLAB_URL`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET` 環境變數說明與 Chat UI 使用方式
- [x] 6.2 更新 `Dockerfile`：確認 GitLab OAuth 所需環境變數可透過 `-e` 注入
- [x] 6.3 更新 `docs/test-cases.md`：新增 T44（GitLab login）、T45（Chat 查詢 + tool call 展開）、T46（多使用者並行 session）
- [x] 6.4 執行 `go test ./...` 確認全部通過
