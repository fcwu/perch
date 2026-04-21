## 1. ACP Client

- [x] 1.1 新增 `acp_client.go`，定義 `ACPClient` struct 與 `NewACPClient(baseURL string)` constructor
- [x] 1.2 實作 `CreateRun(ctx, message, metadata)` — 送出 `POST /runs` 並回傳 run ID
- [x] 1.3 實作 `StreamRun(ctx, runID)` — 連線 `GET /runs/{id}/stream` SSE，回傳 `<-chan string` 文字 chunk channel
- [x] 1.4 在 SSE 解析中處理 `MessageOutput`、`RunCompleted`、`RunFailed` 事件類型
- [x] 1.5 實作 context cancellation：caller cancel 時關閉 SSE 連線並回傳錯誤
- [x] 1.6 讀取 `ACP_BASE_URL` 環境變數，於 `main.go` 初始化時建立 `ACPClient`（未設定則為 nil）
- [x] 1.7 為 ACP client 加入單元測試（mock HTTP server 驗證 create run + SSE streaming）

## 2. IMAdapter 介面調整

- [x] 2.1 新增 `IMConfig` struct（欄位：`PTYManager *PTYManager`、`ACPClient *ACPClient`）
- [x] 2.2 修改 `IMAdapter` 介面：`Start(*PTYManager)` → `Start(cfg IMConfig)`
- [x] 2.3 更新 `IMManager.Start()` 呼叫端以傳入 `IMConfig`
- [x] 2.4 更新 Telegram adapter 實作以符合新介面（Telegram 忽略 ACPClient）

## 3. Discord ACP Session

- [x] 3.1 在 `discordSession` 新增 `acpClient *ACPClient` 欄位
- [x] 3.2 在 `onMessage()` 中判斷 `acpClient != nil`：是則走 ACP 路徑，否則走現有 PTY 路徑
- [x] 3.3 實作 ACP 路徑：`CreateRun()` → 加 👀 reaction → `StreamRun()` 累積輸出 → 完成後送 Discord 訊息
- [x] 3.4 ACP 路徑加入 run timeout（預設 5 分鐘，可透過 `ACP_RUN_TIMEOUT` 環境變數覆寫）
- [x] 3.5 ACP 路徑的 emoji 狀態：成功更新為 💬，失敗更新為 ❌，timeout 更新為 ❌ 並送錯誤訊息
- [x] 3.6 在 ACP 路徑重用現有輸出格式函式（`splitForDiscord`、`convertTablesToCodeBlocks` 等）
- [x] 3.7 空輸出不送 Discord 訊息

## 4. 移除 PTY 相依（ACP 模式）

- [x] 4.1 確認 `discordSession.pty`、`warm`、`watcherCancel` 欄位在 ACP 模式下不被初始化
- [x] 4.2 確認 ACP 模式下不呼叫 PTY warm-up 偵測邏輯
- [x] 4.3 Hook event routing（`/hook`）在 ACP 模式下不再對 Discord session 發送通知（避免重複回覆）

## 5. 測試與驗證

- [x] 5.1 端對端測試：啟動 mock ACP server，驗證 Discord message → ACP run → Discord reply 完整流程
- [x] 5.2 驗證 `ACP_BASE_URL` 未設定時，Discord 正常 fallback 到 PTY 模式
- [x] 5.3 驗證 DM allowlist 在 ACP 模式下仍正確過濾非授權用戶
- [x] 5.4 驗證 run timeout 觸發後 Discord 收到 ❌ reaction 與錯誤訊息
- [x] 5.5 驗證 ACP server 不可達時 Discord 收到適當的錯誤回應

## 6. 清理（PTY 程式碼移除，Phase 2）

- [ ] 6.1 確認 ACP 模式穩定後，移除 `discordSession` 的 PTY 相關欄位與 warm-up 邏輯
- [ ] 6.2 評估是否從 `IMAdapter.Start()` 移除 PTYManager 參數（若所有 IM adapter 都不再需要）
- [x] 6.3 更新 `CLAUDE.md` 記錄 ACP 模式的設定方式

## 7. ~~HTTP ACP client 修正~~（已廢棄，由 Section 9 取代）

> Section 7 和 8 實作了基於 HTTP bridge 的方案，但設計已改為 stdio subprocess 方案。
> 這些任務的程式碼（`acp_client.go`）將在 Section 9 中被移除。

- [x] ~~7.1~~ 已廢棄
- [x] ~~7.2~~ 已廢棄
- [x] ~~7.3~~ 已廢棄
- [x] ~~7.4~~ 已廢棄
- [x] ~~7.5~~ 已廢棄
- [x] ~~7.6~~ 已廢棄（CLAUDE.md 將在 9.15 重新更新）

## 8. ~~trust-all-tools（HTTP 版本）~~（已廢棄，由 Section 9 取代）

- [x] ~~8.1~~ 已廢棄
- [x] ~~8.2~~ 已廢棄（改為 permissionMode: bypassPermissions）
- [x] ~~8.3~~ 已廢棄（CLAUDE.md 將在 9.15 重新更新）

## 9. ACP stdio subprocess 實作（正確方案）

> 移除 HTTP-based `acp_client.go`，改為 Perch 直接管理 `claude-agent-acp` subprocess，透過 ACP JSON-RPC over stdio 通訊。

### 9a. ACPProcess — subprocess 管理與 JSON-RPC 協議

- [x] 9.1 新增 `acp_process.go`：定義 `ACPProcess` struct（含 `executable`、`workdir`、`cmd`、`stdin`、`stdout`、`sessionID`、`mu`、`nextID` 欄位）
- [x] 9.2 實作 subprocess fork：`exec.Command(executable)` + `stdin/stdout pipes`（非 PTY）
- [x] 9.3 實作 ACP JSON-RPC 讀寫底層：`sendRequest(method, params)` 送 JSON 行、`readMessages()` goroutine 持續讀 stdout 分發 response/notification
- [x] 9.4 實作 `Start(ctx)`：fork process → 送 `initialize` request → 送 `new_session`（帶 `permissionMode: "bypassPermissions"`）→ 儲存 `sessionID`
- [x] 9.5 實作 `Prompt(ctx, text) (string, error)`：送 `prompt` request（帶 sessionID）→ 累積 `agent_message_chunk` notifications → 等待 `RunCompleted` / `RunFailed`
- [x] 9.6 實作 context cancellation：ctx.Done() 時關閉 subprocess stdin，確保 goroutine 退出
- [x] 9.7 實作 subprocess crash 偵測：`cmd.Wait()` goroutine 監控，process 退出後設 `running = false`
- [x] 9.8 實作 `EnsureRunning(ctx)`：若 process 未運行則重新 `Start()`（用於 crash auto-recovery）

### 9b. Discord session 整合

- [x] 9.9 修改 `im_discord.go`：`discordSession.acpClient *ACPClient` → `discordSession.acpProcess *ACPProcess`
- [x] 9.10 修改 `im_discord.go`：`handleWithACP()` 改呼叫 `acpProcess.EnsureRunning()` + `acpProcess.Prompt()`
- [x] 9.11 修改 `im_discord.go`：`newDiscordSession()` 在 ACP 模式下初始化 `ACPProcess`（不 Start，lazy init）

### 9c. 環境變數與啟動流程

- [x] 9.12 修改 `main.go`：讀取 `DISCORD_ACP_ENABLED=true` 取代 `ACP_BASE_URL`；移除 `NewACPClient()` 呼叫
- [x] 9.13 修改 `im.go`：`IMConfig.ACPClient *ACPClient` → `IMConfig.ACPEnabled bool`（或移除 ACPClient 欄位）
- [x] 9.14 修改 `im_telegram.go`：對應 IMConfig 欄位名稱變更

### 9d. 移除舊 HTTP 實作

- [x] 9.15 刪除 `acp_client.go`（HTTP-based ACP client，整個方向錯誤）
- [x] 9.16 刪除 `acp_client_test.go`（對應 HTTP client 的測試）

### 9e. 測試

- [x] 9.17 新增 `acp_process_test.go`：用 `io.Pipe()` 建立 fake subprocess stdio，驗證 JSON-RPC initialize/new_session/prompt 流程
- [x] 9.18 更新 `im_discord_acp_test.go`：mock `ACPProcess`（實作 `Prompt()` interface）取代 mock HTTP server
- [x] 9.19 驗證 `DISCORD_ACP_ENABLED` 未設定時，Discord 正常 fallback 到 PTY 模式
- [x] 9.20 驗證 subprocess crash 後下一則訊息自動重啟並繼續處理

### 9f. 文件更新

- [x] 9.21 更新 `CLAUDE.md` Section 4：改為 ACP stdio 模式說明（`DISCORD_ACP_ENABLED`、`ACP_EXECUTABLE`、`ACP_RUN_TIMEOUT`、安裝 `claude-agent-acp`）
