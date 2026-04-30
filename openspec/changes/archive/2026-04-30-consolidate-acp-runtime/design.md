## Context

perch 的 agent runtime 目前是 PTY + hook 與 ACP 兩套並存：

- **PTY + hook（舊）**：透過 Claude Code CLI 起 PTY，用 `claude/settings.json` 註冊 PreToolUse / PostToolUse / Stop hook，每個事件 `curl POST /hook`，perch 端在 `hook.go` 把事件 fan-out 到 `IMManager.notify()`、`UserSessionManager.NotifyHook()`、`AdminHub`、`query_log_store`。Discord PTY fallback、Telegram、chat-API（`claude -p`）走這條。
- **ACP（新）**：透過 stdio JSON-RPC 與 `@agentclientprotocol/claude-agent-acp` subprocess 互動，事件結構化（`agent_message_chunk`、`tool_call_started`、`tool_call_completed`、`RunCompleted`），不依賴 hook、不依賴 PTY 解析、不卡 trust dialog。Discord ACP mode（預設）走這條。

並存代價：

- **同樣的 admin observability 邏輯有兩個 source**：tool event、session lifecycle、query log 寫入要分別接 hook 與 ACP event。
- **PTY 路徑脆弱**：warm-up 偵測、ANSI parsing、Stop hook race（QA 報告 MT07/T52 就是 race）。
- **entrypoint 維護成本**：`claude/settings.json`、`merge-settings.js`、user-level merge workaround 都是為了讓 PTY 路徑載到 hook。
- **Claude Code 2.1.x 升級摩擦**：interactive trust/onboarding 阻塞只發生在 PTY；ACP 走 SDK 沒這些。

QA 報告 `tests/test-report-2026-04-30-1236-summary.md`「給工程的觀察」第 1 點點明 interactive PTY 不載 project hooks 是 PTY 模型在 Claude Code 2.1.x 的死結。把所有 IM 與 chat-API 統一到 ACP 才是根本解。

## Goals / Non-Goals

**Goals:**

- Discord、Telegram、chat-API 統一走 ACP；移除 PTY fallback 與 hook 系統
- Admin observability（live、history、tool-call-stream、query log）的事件來源從 hook → ACP event handler，行為不退步
- 移除 `hook.go`、`/hook` endpoint、`HookEvent` 結構、`claude/settings.json`、`claude/merge-settings.js`、entrypoint hook merge 步驟
- 保留 Web `/ws` 主終端機（單一全域 PTY，使用者直接打 claude CLI），它與 IM/chat-API 無關，獨立於 ACP wiring
- 一套 runtime 模型、一套故障模式、一套測試套件

**Non-Goals:**

- 改寫 Web `/ws` 主終端機（保留 PTY）
- 改寫 ACP client 既有實作（`acp_client.go`、Discord ACP session 邏輯）
- 改 ACP protocol 本身或 `@agentclientprotocol/claude-agent-acp` 套件
- 改 sqlite schema（query_sessions、tool_events、conversations 表結構不變，只改寫入時機）
- 改 frontend chat UI 或 admin live UI（後端事件 source 換掉，前端 wire protocol 不變；只在 admin 命名 rename 時有變動）
- 處理 `PERCH_MODE=single` vs `multi` 的 admin/live access control 改動（與本 change 無關）

## Architecture

### 事件流現況（hook 派 + ACP 派並存）

```
chat-API:   /api/chat ──► spawn `claude -p` (PTY drain)
                            │
                            ├─► stdout → SSE/WS to browser
                            │
                            └─► hooks → POST /hook ──► hook.go fan-out:
                                                         ├─► IMManager.notify()        (no-op for chat-API)
                                                         ├─► UserSessionManager.NotifyHook()
                                                         │     ├─► AdminHub.SessionAdded/Updated/Removed
                                                         │     └─► query_log_store INSERT/UPDATE
                                                         └─► (其他 consumers)

Discord PTY fallback (DISCORD_ACP_ENABLED=false):
            Discord msg ──► PTY write ──► claude (PTY) ──► /hook ──► im_discord.notify() reaction

Discord ACP (預設):
            Discord msg ──► ACP CreateRun ──► subprocess ──► RunCompleted ──► reply

Telegram:
            Telegram msg ──► PTY write ──► claude (PTY) ──► /hook ──► telegram.notify()
```

### 事件流改後（ACP only）

```
chat-API:   /api/chat ──► acp_client.NewSession + Prompt ──► subprocess
                            │
                            ├─► agent_message_chunk → SSE/WS to browser
                            │
                            └─► tool_call_started/completed, RunCompleted
                                  ├─► AdminHub.SessionAdded/Updated/Removed
                                  └─► query_log_store INSERT/UPDATE/finalize

Discord:    Discord msg ──► ACP CreateRun ──► subprocess ──► RunCompleted ──► reply
            tool events ──► AdminHub (optional, see Q3)

Telegram:   Telegram msg ──► ACP CreateRun ──► subprocess ──► RunCompleted ──► reply
            tool events ──► AdminHub (optional, see Q3)

Web /ws:    (unchanged) browser ←WS→ s.pty (single shared PTY) ←→ interactive claude
            (no hook, no admin observability — it's a raw debug terminal)
```

### Subprocess lifecycle

- **chat-API**：每個 user × conversation 一個 ACP subprocess，conversation 結束（idle timeout 或顯式關閉）後回收。多輪對話復用同一 subprocess（取得 ACP `new_session` 持久 session ID）。
- **Discord**：per-channel ACP subprocess（既有，保持）。
- **Telegram**：per-chat（個人 chat 或 group chat）ACP subprocess。
- 共用 idle timeout、subprocess 異常重啟邏輯，抽出 `acp_session_pool`（如果尚未抽出）。

## Decisions

### D1：chat-API 從 `claude -p` 改 ACP per-conversation 持久 session，不每次重 spawn

**決策**：`/api/chat` 收到 query 時，從 `(user_id, conversation_id)` 取得（或新建）一個 ACP subprocess，呼叫 `prompt(sessionID, text)` 提交 query，stream `agent_message_chunk` 與 tool events 回 browser，`RunCompleted` 後 subprocess 留著等下一個 prompt（idle timeout 或 conversation 結束才終止）。

**替代方案：**

- *每筆 query spawn 一個 ACP subprocess、答完即殺*：行為等同舊 `claude -p`，但暖機 overhead × N 倍。
- *全使用者共用一個 subprocess*：state pollution；ACP `new_session` 設計就是要分開。
- *維持 `claude -p` 模式但用 ACP runner wrap*：本質上沒解 PTY drain race。

**理由**：

- ACP 的 `new_session` 與 `prompt` API 設計就是 per-session 持久；用其本意。
- 多輪對話免 re-prepend history（這是 chat-API 改 ACP 的最大實質收益）。
- subprocess 池 + idle timeout 簡單，與 Discord ACP 結構一致。

### D2：Telegram 改 ACP 用 per-chat 模型，與 Discord 對齊

**決策**：Telegram per-chat（不論個人 / group）一個 ACP subprocess。`im_telegram.go` 重寫類比 `discord_acp_session.go`：subprocess pool、ACP CreateRun、stream → 回 chat 訊息、reactions / typing indicator。

**替代方案：**

- *直接刪 Telegram*：使用者選 A，要保留。
- *Telegram 走 chat-API ACP 那層*：兩條 IM 模型不一致，徒增複雜。

### D3：移除 hook 系統的步驟（cleanup 順序）

**決策**：

1. 先把 chat-API、Telegram、Discord 全部接到 ACP（含 admin observability 改用 ACP event）。
2. 確認沒有路徑寫入 `/hook` 後，刪 `hook.go`、`/hook` 路由、`HookEvent` struct、`IMAdapter.Notify(HookEvent, string)` 介面方法。
3. 刪 `claude/settings.json`、`claude/merge-settings.js`，以及 entrypoint.sh 中呼叫 `merge-settings.js` 的 block。
4. 移除 image 中 `/app/perch-claude/settings.json`、`/app/perch-claude/merge-settings.js`（Dockerfile 與 build script）。

**理由**：刪 hook 之前必須確認所有 consumer 都搬走，否則 admin live/history 會靜默壞掉。

### D4：Admin observability 從 hook 路由 → ACP event handler

**決策**：在 `acp_client` 內部對每個 ACP run 註冊 callback：`tool_call_started` → `AdminHub.SessionUpdated(sessionID, toolName)`、`tool_call_completed` → `query_log_store.UpdateToolEvent(...)`、`RunCompleted` → `AdminHub.SessionRemoved` + `query_log_store.UpdateSession(...)`。session 在 `Prompt` 開始時 `AdminHub.SessionAdded` + `query_log_store.InsertSession`。

**替代方案：**

- *新建一層 event router 把 ACP event 翻譯成 HookEvent struct，hook.go 不動*：擋住一半的好處，hook handler 還在；不採。

**理由**：直接接 ACP event 才能拿到結構化資料（tool input/output 都是 JSON object，不必再 marshal/unmarshal），AdminHub 與 query_log_store 的 update method 簽章可以更乾淨。

### D5：Web `/ws` 主終端機保留 PTY，獨立於本 change

**決策**：`s.pty`（`/ws` handler 對應的 single shared PTY）保留現狀，不改 ACP。它與 IM/chat-API 無關、不依賴 hook、是使用者直接看 claude CLI 的「除錯介面」。

**理由**：

- `/ws` 的核心價值是「看到 claude CLI 原生輸出」，ACP 結構化事件反而失去 raw terminal 觀感。
- 移除它會少一個 power-user 工具，沒有對應利益。
- 它不參與 admin observability，不影響 hook 移除。

### D6：`DISCORD_ACP_ENABLED` 環境變數移除

**決策**：刪除 `DISCORD_ACP_ENABLED`。Discord 永遠走 ACP。配套：`im_discord.go` 內 `pty *PTYManager` 欄位、`pty` 路徑分支、warm-up 邏輯、PTY watcher 全部移除。

**替代方案：**

- *保留旗標 default true，但 false 不生效（log warning 並走 ACP）*：表達不夠強，使用者 config 仍能誤設。

**理由**：fallback 清乾淨，避免使用者誤以為還能切回 PTY。

### D7：admin observability data model 不變，只換 source

**決策**：`AdminHub`、`AdminSessionView`、`query_sessions` 表、`tool_events` 表、`/api/admin/history` response schema、`/ws/admin` event JSON schema 都不變（除非走 D8 的 admin → management rename）。本 change 只換**事件來源**，不重設計 admin UX。

**理由**：

- 隔離影響面，避免一次改太多
- 前端 admin 頁面、QA 既有測試（T55/T56）只需要對齊「事件來源」，不需要對齊 schema 變動

### D8：admin → management rename（**已決：併入本 change**）

**決策**：本 change 範圍涵蓋路徑、struct、middleware、frontend page、capabilities 一次性 rename：

- 路徑：`/api/admin/*` → `/api/management/*`、`/ws/admin` → `/ws/management`
- Middleware：`adminMW` → `managementMW`
- Go struct：`AdminHub` → `ManagementHub`、`AdminSessionView` → `ManagementSessionView`、`adminEvent` → `managementEvent`
- Handlers：`handleAdminWS` → `handleManagementWS`、`handleAdminHistory(Detail)?` → `handleManagementHistory(Detail)?`
- Frontend：`AdminPage` → `ManagementPage`
- Capabilities：`admin-realtime` → `management-realtime`、`admin-history` → `management-history`

**理由**：本 change 反正要重寫 admin observability 的事件來源（hook → ACP），趁路徑重寫一次到位 rename，比後續單獨開 rename change 影響面更小。Breaking change 集中釋出比分散 disruption 好。

### D9：Live access 限 multi-user mode（**已決**）

**決策**：`/ws/management` Live endpoint 僅在 `PERCH_MODE=multi` 啟用：single-user mode 下 `server.go` 不註冊該路由，請求回 404。對應 spec capability `management-realtime` 加 ADDED Requirement 描述此 access gate。

**替代方案：**

- *完全砍 Live*：簡化 codebase 但失去 multi-user 場景的監控價值。
- *保留 Live 在所有 mode*：single-user 模式下使用者監控自己 query，無實質價值；徒增 ws hub 複雜度。

**理由**：

- single-user mode 下使用者就是 admin 自己，看自己 live query 沒意義
- multi-user mode 下 admin 監控團隊成員 query 有 surveillance / 除錯 / 容量規劃 / capacity 價值
- History 功能在所有 mode 都保留（事後查詢有價值）
- 實作成本低（路由註冊處加 `if mode == "multi"`），可逆性高（未來想開放到 single 容易）

## Risks / Trade-offs

- **chat-API ACP subprocess pool 記憶體佔用**：每個 user × conversation 一個 subprocess（claude-agent-acp + Claude Code SDK）。若使用者開很多 conversation，記憶體會上升。需 idle timeout（建議 10–15 分鐘）+ pool 上限（每 user N 個）。
- **chat-API 多輪 latency 改善**：persistent subprocess 不再 cold start，第二輪以後 latency 應顯著下降（節省 claude CLI 啟動時間）。
- **Telegram 改寫風險**：Telegram 使用者目前依賴 PTY 行為（典型訊息回覆延遲、reaction 模式），ACP 化後體驗會略有差異（emoji 狀態時序、訊息分割）。需要使用者驗收。
- **admin observability event source 切換**：T55/T56 既有測試依賴 hook 觸發；切到 ACP 之後，event 順序可能微調（例如 `tool_call_started` 與 PTY 顯示文字的相對 timing），測試 assertion 要對齊。
- **hook 移除影響第三方 integrations**：若有人靠 `/hook` endpoint 自製 webhook，會失效。release note 必須標 breaking change。
- **`claude -p` 完全不用後**：image 仍含 claude CLI（web `/ws` 用），但 chat-API 不再 spawn `claude -p`，runtime selection 簡化。OpenCode runtime 的 chat-API 路徑要對應決定（仍 spawn `opencode run -p` 還是也 ACP 化？暫保留現狀）。
- **subprocess 暖機 vs 互動回覆**：第一次 `prompt` 時 ACP subprocess 仍要啟動 + initialize，使用者第一句訊息會比 cold-start 之後的訊息慢。建議在 chat-API 接到 request 但 subprocess 未起時，先給 browser 一個 "starting agent..." 狀態 chunk。

## Migration Plan

實作建議按以下順序，每步可獨立驗證：

1. **Phase 1：Telegram 改 ACP**（最小爆炸半徑，是新 capability，不破壞既有功能）
   - 寫 `im_telegram_acp.go` 類比 `im_discord_acp` 結構
   - 切換 `im_telegram.go` 主入口走 ACP
   - 跑 Telegram 測試（如有）

2. **Phase 2：chat-API 改 ACP**
   - 在 `user_session.go` 旁邊新建 `chat_api_acp.go`，實作 ACP-based session lifecycle
   - 灰階切換：環境變數 `CHAT_API_ACP_ENABLED=true`（預設 false）開啟新路徑
   - 雙模式並行跑 MT01-12、T52 確認等價
   - 切預設為 true，移除 `claude -p` 路徑

3. **Phase 3：Discord PTY fallback 移除**
   - `im_discord.go` 刪 PTY 分支與 `DISCORD_ACP_ENABLED` flag
   - 跑 T18/T19/T46 確認

4. **Phase 4：Admin observability 切 ACP event**
   - `acp_client` 內接 callback 到 `AdminHub` + `query_log_store`
   - hook handler 在驗證後仍保留作為 safety net；雙寫一個版本確認等價，再砍 hook
   - 跑 T55/T56/MT12 確認

5. **Phase 5：Hook 系統移除**
   - 刪 `hook.go`、`/hook`、`HookEvent`、`IMAdapter.Notify`
   - 刪 `claude/settings.json`、`claude/merge-settings.js`、entrypoint merge step
   - Image 重 build，跑全 batch 確認

6. **Phase 6：Cleanup**
   - 刪 `runtime.go` 中 chat-API 用的 `claude -p` arg builder（如已沒人用）
   - 刪 `PTYManager` 對 IM 的 wiring（保留 web `/ws` 用的 single PTY）
   - 文件更新

每個 phase 都應有對應的 git commit + QA cycle 跑測試。

## Open Questions

_All resolved._

- ~~**Q1**：admin → management rename 併入本 change？~~ **已決**：併入（D8）
- ~~**Q2**：Live 取捨？~~ **已決**：B（限 multi-user 啟用，D9）
- ~~**Q3**：Discord / Telegram 的 tool event 是否進 ManagementHub？~~ **已決**：不進。只 chat-API 進 ManagementHub，維持既有 admin observability scope（IM 是訊息驅動、不是 query 驅動，混進來反而雜）。
- ~~**Q4**：ACP subprocess pool 上限數值？~~ **已決**（初始預設值，實測後可調）：per-user 上限 5、global 上限 50、idle timeout 15 分鐘。
- ~~**Q5**：OpenCode runtime 的 chat-API 也 ACP 化？~~ **已決**：本 change 只處理 Claude；OpenCode 留給後續另開 change。
- ~~**Q6**：Phase 5 hook 移除前留過渡 release？~~ **已決**：不留，兩個 change（`fix-claude-code-container-compat` + `consolidate-acp-runtime`）是同一架構演進的兩步，順序實作即可。
