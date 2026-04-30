## Why

perch 目前有兩套 agent runtime 模型並存：

1. **PTY + hook**：Discord PTY fallback、Telegram、chat-API（`claude -p`）。透過 `claude/settings.json` 的 hook 把 PreToolUse/PostToolUse/Stop POST 回 `/hook` endpoint，再分發到 `IMManager.notify()`、`UserSessionManager.NotifyHook()`、`AdminHub`、`query_log_store`。
2. **ACP**：Discord ACP mode（預設，2026-04-21 加入）。透過 stdio JSON-RPC 與 `claude-agent-acp` subprocess 互動，事件結構化、無 trust dialog/theme dialog 阻塞。

兩套並存帶來問題：
- 同樣的 admin observability（live、history、query log）邏輯有兩個 source（hook 與 ACP event），維護成本翻倍
- PTY 路徑的脆弱（warm-up 偵測、ANSI parsing、Stop hook race，QA 報告 MT07/T52 就是 race 案例）持續發生在 chat-API 與 Telegram
- entrypoint 的 hook merge 步驟（與 `fix-claude-code-container-compat` 中暫保留的 user-level merge 需求）只是為了讓 PTY 路徑載到 hook，是 PTY 模型的副作用
- Claude Code 2.1.x 的 interactive trust/onboarding 阻塞只發生在 PTY 路徑（ACP 走 SDK，沒有這些阻塞），併入 ACP 後相關 workaround 都不必要

把所有 agent 互動統一到 ACP，可以：
- 移除 hook 系統、`hook.go`、`/hook` endpoint、`claude/settings.json`、`claude/merge-settings.js`、entrypoint hook merge 步驟
- chat-API admin observability 重新接到 ACP event（per-session subprocess 的 lifecycle 與 tool event 都是 first-class 結構化事件）
- 移除 `PTYManager` 在 chat-API、Discord、Telegram 路徑的 wiring（保留 web `/ws` 主終端機作為直連 PTY，獨立於 IM/chat-API）
- 一套 runtime 模型、一套測試、一套故障模式

對應 QA 報告：`tests/test-report-2026-04-30-1236-summary.md` 中「給工程的觀察」第 1 點點明 interactive PTY 不載 project hooks，是 PTY 模型在 Claude Code 2.1.x 的死結；走 ACP 是根本解。

## What Changes

- **移除** Discord PTY fallback：`im_discord.go` 中 `DISCORD_ACP_ENABLED=false` 分支整段刪除，Discord 永遠走 ACP。`PTYForTarget` 對 Discord target 不再回傳 PTY。`DISCORD_ACP_ENABLED` 環境變數移除（或保留但無效）
- **遷移** Telegram 到 ACP：`im_telegram.go` 改用 `acp_client`（per-chat 或 per-user subprocess，類比 Discord 的 per-channel），移除對 `PTYManager` 的依賴。`Notify(HookEvent, string)` 介面從 `IMAdapter` 移除（沒有 hook 來源了）
- **遷移** chat-API 到 ACP：`user_session.go` 中 `RunAgent` 起的 `claude -p` 改用 `acp_client.CreateRun()` + `StreamRun()`。session lifecycle 由 ACP `new_session` / `RunCompleted` 驅動，不再依賴 Stop hook。AdminHub 事件來源改為 ACP event handler
- **遷移** scheduler 觸發路徑：scheduler 現在透過 `IMAdapter` 把 schedule 訊息送進對應的 IM session（已是 ACP 路徑），只需確認 telegram 遷移後不需要 PTY
- **移除** hook 系統：刪除 `hook.go`、`/hook` HTTP endpoint、`HookEvent` 結構、`IMAdapter.Notify(HookEvent, string)` 方法、`UserSessionManager.NotifyHook()` 方法、`hookHandler`、`im.notify()` dispatch
- **移除** `claude/settings.json` 與 `claude/merge-settings.js`（perch hooks 不再 inject）
- **移除** `entrypoint.sh` 的 hook merge 步驟（呼叫 `merge-settings.js` 那段）
- **修改** `query_log_store` 寫入時機：從 Stop hook → ACP `RunCompleted` event handler；workspace 內仍寫到 sqlite，schema 不變
- **修改** `tool-call-stream` 事件來源：從 hook PreToolUse/PostToolUse → ACP `tool_call_started` / `tool_call_completed` event
- **修改** admin live (`/ws/admin`) 事件來源：從 `UserSessionManager.NotifyHook()` 觸發 → 改為 ACP session handler 觸發 `AdminHub.SessionAdded/Updated/Removed`
- **保留** Web `/ws` 主終端機（單一全域 PTY，使用者直接打 claude CLI）：與 IM/chat-API 無關，繼續存在，但獨立於 ACP wiring
- **保留** ACP client (`acp_client.go`) 與 Discord ACP session 主路徑：本 change 是擴大使用範圍，不重寫
- **修改** entrypoint：原本 `DISCORD_BOT_TOKEN || TELEGRAM_BOT_TOKEN` 才呼叫 `merge-settings.js` 的 block 整個刪除
- **新增** `acp_client` 對 `new_session` 設定 `permissionMode` 之外，可能需要傳 `workspace_path` / `system_prompt` 等 chat-API 專用參數
- **新增** Telegram 的 ACP per-chat session 管理（清理策略、idle timeout、subprocess 重啟）
- **修改** 測試：MT01-MT12（multi-turn-chat）、T19/T18/T46（Discord 系列）、T55/T56（admin → management）、T05-T07（scheduler natural-language）、T52（chat session done）整套對齊 ACP-only 行為。MT07/T52 的 done 事件改驗 ACP `RunCompleted`，不再驗 Stop hook PTY drain
- **重新命名** admin → management：路徑（`/api/admin/*` → `/api/management/*`、`/ws/admin` → `/ws/management`）、Go struct（`AdminHub` → `ManagementHub`、`AdminSessionView` → `ManagementSessionView`）、middleware（`adminMW` → `managementMW`）、frontend page（`AdminPage` → `ManagementPage`）、capabilities（`admin-realtime` → `management-realtime`、`admin-history` → `management-history`）
- **限制** Live (`/ws/management`) 僅在 multi-user mode (`PERCH_MODE=multi`) 啟用：single-user mode 下不註冊路由、回 404；對應 capability 加 access gate Requirement

## Capabilities

### Modified Capabilities
- `acp-client`：擴充 chat-API 與 Telegram 兩種使用情境的 session lifecycle 管理
- `discord-acp-session`：移除 PTY fallback Requirements，純 ACP
- `multi-turn-chat`：runtime 從 `claude -p` 改 ACP，session 持久化於 ACP subprocess
- `tool-call-stream`：事件來源從 hook → ACP event
- `query-log-store`：寫入觸發從 Stop hook → ACP `RunCompleted`
- `user-session-manager`：底層 spawn 從 PTY → ACP

### Renamed Capabilities
- `admin-realtime` → `management-realtime`：事件來源從 hook → ACP event；新增 multi-user-only access gate
- `admin-history` → `management-history`：寫入觸發從 Stop hook → ACP `RunCompleted`

### New Capabilities
- `chat-api-acp`：chat-API per-conversation ACP session pool 與 query streaming
- `telegram-acp-session`：Telegram per-chat ACP session 管理（類比 `discord-acp-session`）
- `acp-tool-events`：ACP event → ManagementHub + query_log_store 路由

### Removed Capabilities
- 無正式 capability 名稱叫「hook」的，但 spec 內依賴 hook 的 Scenario 都會被改寫；hook 的實作概念整體移除

## Impact

- `im_discord.go`：刪除 PTY fallback 程式碼路徑、`DISCORD_ACP_ENABLED` flag、`pty *PTYManager` 欄位
- `im_telegram.go`：整個改寫為 ACP-based；可能拆出 `telegram_acp_session.go` 對齊 Discord 結構
- `user_session.go`：`RunAgent` 改 ACP 呼叫；hook handler 移除；session lifecycle 重寫
- `hook.go`：**整個檔案刪除**
- `im.go`：`IMAdapter.Notify(HookEvent, string)` 介面移除；`HookEvent` 結構移除；`IMManager.notify()` 移除
- `server.go`：`/hook` 路由移除；`hookHandler` 註冊移除
- `runtime.go`：`RunAgent` 對 Claude 的 `claude -p` 模式可能整段移除（chat-API 與 IM 都不再用），保留 OpenCode 路徑與 web `/ws` 用的 interactive
- `acp_client.go`：可能新增 chat-API 與 Telegram 專用的 helper（單次 query vs 持久 session）
- `entrypoint.sh`：移除 `claude/merge-settings.js` 呼叫整段；簡化為 cp + onboarding seed（與 `fix-claude-code-container-compat` 配合）
- `claude/settings.json`、`claude/merge-settings.js`、`/app/perch-claude/` 的 hook 相關檔案：**全部刪除**
- 環境變數：移除 `DISCORD_ACP_ENABLED`（或保留但 ignored，並 deprecation warning）
- 測試：MT01-MT12、T19、T18、T46、T55、T56、T52、T07、T34-T43 整套需要 update 對齊 ACP-only 行為
- 文件：CLAUDE.md「容器除錯」段移除 hook 相關 troubleshooting；README 移除 hook env 設定段；新增 ACP-only 架構說明
- **Breaking change for existing deploys**：`DISCORD_ACP_ENABLED=false` 不再有效；任何依賴 `/hook` endpoint 自製 integration 失效

## Dependencies / Sequencing

- 本 change 應在 `fix-claude-code-container-compat` **之後**實作（後者把 entrypoint 與 mount 換到 cp 模型，本 change 在那基礎上把 hook merge 刪掉）
- 本 change 完成後可以再開一個 cleanup change 把 `PTYManager` 對 IM 的 wiring 徹底移除（保留 web `/ws` 主終端機用），但不在本 change scope
