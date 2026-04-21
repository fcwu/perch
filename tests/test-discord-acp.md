# Discord ACP 整合 測試案例

> 功能：discord-acp
> 涵蓋範圍：PTY fallback、DM allowlist in ACP mode、run timeout、ACP server unreachable。
> 相關任務：tasks 5.2–5.5（Discord ACP mode）。
> 撰寫日期：2026-04-21

---

## T63 — 未設 ACP_BASE_URL 時維持 PTY 模式

**層級**：Unit

> **自動化**：`go test -run TestDiscordPTYFallback ./...`

**Given** Perch 啟動時未設定 `ACP_BASE_URL` 環境變數
**When** `DiscordSessionManager` 透過 `Start()` 初始化，且 `IMConfig.ACPClient` 為 nil
**Then**
- `discordSession.acpClient` 欄位為 nil
- `discordSession.pty` 欄位為非 nil（PTY 已啟動）
- `DiscordSessionManager` 日誌顯示 "Discord bot connected (PTY mode)"

**When** 使用者傳送 Discord 訊息後
**Then** 訊息透過 PTY 寫入（`sess.pty.write` 被呼叫），不走 `handleWithACP` 路徑

---

## T64 — 設定 ACP_BASE_URL 後進入 ACP 模式，不啟動 PTY

**層級**：Unit

> **自動化**：`go test -run TestDiscordACPMode ./...`

**Given** 建立一個 `discordSession`，傳入非 nil 的 `*ACPClient`
**When** `newDiscordSession` 完成初始化
**Then**
- `discordSession.acpClient` 為非 nil
- `discordSession.pty` 為 nil（ACP 模式下不啟動 PTY 子行程）

**When** `DiscordSessionManager.Start()` 以非 nil `ACPClient` 呼叫
**Then** 日誌顯示 "Discord bot connected (ACP mode)"，且不預先為 allowedChannelID 建立 PTY session

---

## T65 — ACP 模式下 DM Allowlist 正確過濾未授權使用者

**層級**：Unit

> **自動化**：`go test -run TestDiscordACPDMAllowlist ./...`

**Given** Perch 以 ACP 模式啟動（`ACP_BASE_URL` 已設），且 `DISCORD_ALLOWED_USER_IDS` 只包含使用者 ID `"111"`
**When** Discord 使用者 ID `"999"`（不在 allowlist）傳送 DM 訊息給 Bot
**Then**
- `onMessage` handler 在 allowlist 檢查後直接 return
- ACP client 的 `CreateRun` 未被呼叫
- 該使用者的訊息無任何 reaction 或回應

**When** Discord 使用者 ID `"111"`（在 allowlist）傳送 DM 訊息
**Then** 訊息進入 `handleWithACP` 處理流程，ACP `CreateRun` 被呼叫

---

## T66 — ACP 模式：正常完成流程（Happy Path）

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPHappyPath ./...`

**Given** 一個 mock ACP 伺服器（`httptest.NewServer`）：
- `POST /runs` 回傳 `{"run_id": "run-abc"}`
- `GET /runs/run-abc/stream` 依序送出 `MessageOutput`（"Hello!"）、`RunCompleted` SSE 事件

**When** Discord 使用者傳送訊息觸發 `handleWithACP`

**Then** 依序觀察到：
1. Discord 訊息上出現 👀 reaction
2. `CreateRun` 收到正確的訊息內容與 `discord_channel_id` metadata
3. 👀 reaction 被移除
4. 訊息上出現 💬 reaction
5. Bot 以 reply 形式在同一頻道回覆 "Hello!"

---

## T67 — ACP 伺服器無法連線：Discord 收到 ❌ 與錯誤訊息

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPServerUnreachable ./...`

**Given** `ACP_BASE_URL` 指向一個不存在的位址（如 `http://127.0.0.1:19999`）
**When** Discord 使用者傳送任意訊息
**Then**
1. Discord 訊息上先出現 👀 reaction
2. `CreateRun` 因連線失敗回傳錯誤
3. 👀 reaction 被移除，改出現 ❌ reaction
4. Bot 在該頻道以 reply 形式回覆，內容以 `❌ Agent unavailable:` 開頭，後接錯誤描述

**反向驗證**：回覆訊息不應包含 💬 reaction（因為從未成功取得 run 結果）。

---

## T68 — ACP 伺服器回傳 HTTP 5xx 錯誤：Discord 收到 ❌

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPServerError ./...`

**Given** mock ACP 伺服器的 `POST /runs` 回傳 HTTP 500
**When** Discord 使用者傳送訊息觸發 `handleWithACP`
**Then**
1. 訊息上先出現 👀 reaction
2. 👀 被移除，改出現 ❌ reaction
3. Bot 回覆以 `❌ Agent unavailable:` 開頭，包含 "500" 字串

---

## T69 — Run 超時：Discord 收到 ❌ 與逾時提示

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPRunTimeout ./...`

**Given** mock ACP 伺服器：
- `POST /runs` 正常回傳 run ID
- `GET /runs/{id}/stream` 持續保持連線不送任何事件（模擬 hang）

且環境變數 `ACP_RUN_TIMEOUT=2`（2 秒，用於加速測試）

**When** Discord 使用者傳送訊息，2 秒後 context deadline 到期
**Then**
1. 訊息上先出現 👀 reaction
2. 逾時後 👀 被移除，改出現 ❌ reaction
3. Bot 回覆格式為：`❌ ⏱️ Agent timed out.`（固定字串，不含底層錯誤細節）

**反向驗證**：回覆訊息不含 "context deadline exceeded" 等 Go 內部錯誤字串。

---

## T70 — ACP_RUN_TIMEOUT 環境變數正確解析

**層級**：Unit

> **自動化**：`go test -run TestACPRunTimeout ./...`

**Given** 無 `ACP_RUN_TIMEOUT` 環境變數
**When** 呼叫 `acpRunTimeout()`
**Then** 回傳值為 5 分鐘（300 秒）

**When** 設定 `ACP_RUN_TIMEOUT=30` 並呼叫 `acpRunTimeout()`
**Then** 回傳值為 30 秒

**When** 設定 `ACP_RUN_TIMEOUT=abc`（非數字）並呼叫 `acpRunTimeout()`
**Then** 回傳預設值 5 分鐘（fallback 行為，不 panic）

**When** 設定 `ACP_RUN_TIMEOUT=0` 並呼叫 `acpRunTimeout()`
**Then** 回傳預設值 5 分鐘（0 視為無效值，使用 fallback）

---

## T71 — ACP 模式下 Notify() 不處理 ACP Session 的 Hook 事件

**層級**：Unit

> **自動化**：`go test -run TestDiscordACPNotifySkip ./...`

**Given** `DiscordSessionManager` 以 ACP 模式啟動，已有一個 ACP session（`acpClient != nil`）
**When** 外部呼叫 `Notify(HookEvent{EventName: "Stop", ...}, "some text")`
**Then**
- `Notify` 對該 ACP session 不發送任何 Discord 訊息（reply 未送出）
- `Notify` 回傳 nil（不視為錯誤）

（ACP session 自行管理回應流程；Hook 路由不應介入。）

---

## T72 — ACP 模式下 SubscribeSession 回傳 false

**層級**：Unit

> **自動化**：`go test -run TestDiscordACPSubscribeSession ./...`

**Given** `DiscordSessionManager` 以 ACP 模式啟動，已建立一個頻道的 ACP session
**When** 呼叫 `SubscribeSession(channelID)`
**Then** 回傳 `(nil, nil, false)`（ACP session 無 PTY，不支援訂閱）

---

## T73 — ACP 模式下 WriteSession 回傳錯誤

**層級**：Unit

> **自動化**：`go test -run TestDiscordACPWriteSession ./...`

**Given** `DiscordSessionManager` 以 ACP 模式啟動，已有一個 ACP session
**When** 呼叫 `WriteSession(channelID, []byte("hello"))`
**Then** 回傳非 nil 錯誤，錯誤訊息包含 "ACP mode" 字樣

---

## T74 — ACP 模式：RunFailed 事件導致 Discord 收到 ❌

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPRunFailed ./...`

**Given** mock ACP 伺服器：
- `POST /runs` 正常回傳 run ID
- `GET /runs/{id}/stream` 送出 `RunFailed` 事件，payload 包含 `{"error": "agent internal error"}`

**When** Discord 使用者傳送訊息
**Then**
1. 訊息上先出現 👀 reaction
2. 👀 被移除，改出現 ❌ reaction
3. Bot 回覆包含 `❌` 前綴與錯誤描述（"agent internal error"）
4. 不出現 💬 reaction

---

## T75 — ACP 模式：長回應自動切割為多則 Discord 訊息

**層級**：Integration

> **自動化**：`go test -run TestDiscordACPLongReply ./...`

**Given** mock ACP 伺服器回傳一段超過 1900 字元的文字（`RunCompleted` 前送出多個 `MessageOutput`）
**When** Discord 使用者傳送觸發該回應的訊息
**Then**
- 第一則回覆以 Discord reply（附在原訊息）形式送出
- 後續 chunk 以獨立訊息送出在同一頻道
- 每則訊息長度均不超過 1900 字元

---

## T76 — E2E：ACP 模式端對端訊息流程（真實 Discord + mock ACP）

**層級**：E2E-curl

**Given**
- Perch binary 已啟動，設定：`DISCORD_BOT_TOKEN=<token>`、`ACP_BASE_URL=http://localhost:<mock-port>`
- 本機另起一個 mock ACP HTTP 伺服器，回應如 T66 所述
- 使用者已授權 DM（`DISCORD_ALLOWED_USER_IDS` 包含測試帳號 ID）

**When** 測試帳號在 Discord DM 傳送訊息："ping"

**Then**（透過 Discord API 或 bot log 驗證）
1. 目標訊息在 30 秒內出現 👀 reaction
2. 👀 消失，出現 💬 reaction
3. Bot 以 reply 形式回覆 "Hello!"

**反向驗證**：停止 mock ACP 伺服器後重新傳送 "ping"，應在 30 秒內看到 ❌ reaction 及錯誤 reply，不出現 💬。
