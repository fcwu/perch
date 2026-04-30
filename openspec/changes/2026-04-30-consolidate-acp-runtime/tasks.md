## 0. Pre-flight

- [x] 0.1 確認 `fix-claude-code-container-compat` 已合併（容器 cp + onboarding seed 為前置條件） — commit `3037617`（2026-04-30）
- [x] 0.2 解 Open Questions Q1-Q6（見 design.md）— Q1 (rename：併入)、Q2 (Live：限 multi)、Q3 (IM tool event：不進 ManagementHub)、Q4 (pool: per-user 5 / global 50 / idle 15min)、Q5 (OpenCode：另開 change)、Q6 (過渡 release：不留) 全部已決

## 1. ACP session pool（共用基礎建設）

- [x] 1.1 抽出 `acp_session_pool.go`，提供：
  - `Acquire(key string) (*ACPSession, error)`：給定 unique key（如 `chat-api:user42:conv-abc`、`discord:channel:1234`、`telegram:chat:5678`）取得或新建持久 ACP subprocess
  - `Release(key string)`：減 refcount；歸零後啟動 idle timer
  - subprocess crash 自動 cleanup + 下次 Acquire 重啟
- [x] 1.2 加 idle timeout（預設 15 分鐘無 prompt 即終止 subprocess）
- [x] 1.3 加 per-user 上限（預設 5）與 global 上限（預設 50），LRU 淘汰
- [x] 1.4 對 pool 寫 unit test（mock subprocess 模擬 acquire/release/timeout/crash 流程）

## 2. Phase 1：Telegram ACP migration

- [x] 2.1 新建 `im_telegram_acp.go`（類比 `im_discord_acp` 結構）
  - per-chat ACP session（`acp_session_pool` key=`telegram:chat:<chatID>`）
  - 接收 telegram message → ACP `prompt(sessionID, text)` → stream 回 chat
  - typing indicator（送 `sendChatAction`）對應 ACP `tool_call_started`/`agent_message_chunk`
  - 訊息分割（telegram 4096 bytes 上限）
- [x] 2.2 改 `im_telegram.go` 主入口路徑為 ACP（移除 `pty *PTYManager` 欄位）
- [x] 2.3 移除 `TelegramAdapter.Notify(HookEvent, string)` 實作（hook source 不再呼叫）
- [x] 2.4 對 `IMAdapter` 介面移除 `Notify(HookEvent, string) error`（Phase 5 的前置）
- [x] 2.5 撰寫 `tests/test-telegram-acp.md` 測試案例：
  - **TG-A01**：Telegram 個人 chat → ACP run → 回應正確
  - **TG-A02**：Telegram group chat（@提及 bot）→ ACP run
  - **TG-A03**：多輪對話 context 保留（同 chat 兩則訊息共享 ACP session）
  - **TG-A04**：subprocess crash → 下次訊息自動重啟
  - **TG-A05**：idle timeout → subprocess 釋放，下次訊息重啟

## 3. Phase 2：chat-API ACP migration

- [x] 3.1 新建 `chat_api_acp.go`：實作 chat-API 的 ACP-based session manager
  - per-user × conversation 一個 ACP session（key=`chat-api:<userID>:<convID>`）
  - `/api/chat` POST 收到 query：Acquire → `prompt` → stream chunk 回 SSE/WS
  - 設定 ACP `new_session` 帶 `permissionMode: "bypassPermissions"`、`workspace_path`、`system_prompt`（如有）
- [x] 3.2 加環境變數 `CHAT_API_ACP_ENABLED`（預設 false）灰階切換
- [x] 3.3 雙模式並行跑 MT01-12 與 T52 確認等價（PTY 與 ACP 路徑回應同一 query 應該大致等價） — **N/A（路徑未採用）**：PTY chat-API 路徑已於 task 3.4 一次性移除，無雙模式並行階段；MT12 在 batch B 2236 QA 直接以 ACP 行為 PASS（report `tests/test-report-2026-04-30-batchB-2236.md`）
- [x] 3.4 切預設為 true、移除 `CHAT_API_ACP_ENABLED` 旗標、移除舊 `claude -p` 路徑
- [x] 3.5 移除 `runtime.go` 中 Claude 的 `RunAgent` 對 `-p` 模式的 args builder（OpenCode 路徑保留）
- [x] 3.6 改寫測試（不新增 case）：
  - MT01-MT12 對齊 ACP 行為（done event 來自 `RunCompleted` 而非 PTY drain）— 既有 MT01-12 測試文字停留在使用者可見行為層（會話歷史、textarea 解鎖、訊息順序），未明文提到 hook/PTY drain，ACP 行為直接 PASS（batch B 2236 MT12 PASS）
  - T52 chat session done signal 改驗 ACP `RunCompleted` 觸發 textarea 重新啟用 — `tests/test-kb-chat-api.md:87` T52 描述「輸入框恢復可用」，與 ACP `RunCompleted` 行為一致；無需內文修改
  - 既有測試 ID 不變 — ID 全保留

## 4. Phase 3：Discord PTY fallback removal

- [x] 4.1 `im_discord.go` 刪除：
  - `pty *PTYManager` 欄位與相關 wiring
  - `DISCORD_ACP_ENABLED=false` 分支與 PTY warm-up 邏輯
  - PTY watcher goroutine
  - PTY 寫入路徑
- [x] 4.2 移除 `DISCORD_ACP_ENABLED` 環境變數（與其檢查邏輯）
- [x] 4.3 改寫測試（不新增 case）：
  - T18/T46 從「驗 PTY mode」改成 SKIP-as-needs-user-action（PTY mode 不存在）；或 retire 掉 ID
  - T19 reaction state machine 在 ACP 模式下的 emoji 序列驗證仍照舊（既有 PASS）

## 5. Phase 4：Admin observability via ACP event

- [x] 5.1 在 `acp_client` 內為每個 ACP run 註冊 callback：
  - `Prompt` 開始時 → `AdminHub.SessionAdded(...)` + `query_log_store.InsertSession(...)`
  - `tool_call_started` → `AdminHub.SessionUpdated(sessionID, toolName)` + `query_log_store.InsertToolEvent(...)`
  - `tool_call_completed` → `query_log_store.UpdateToolEvent(eventID, output, endedAt)`
  - `RunCompleted` → `AdminHub.SessionRemoved(sessionID, "done")` + `query_log_store.UpdateSession(sessionID, response, endedAt, "done")`
  - `RunFailed` / timeout → `AdminHub.SessionRemoved(sessionID, "error")` + log store status update
- [x] 5.2 雙寫過渡：先讓 hook 與 ACP event 都寫 AdminHub / store，跑 T55/T56/MT12 確認 ACP event 寫入結果與 hook 一致 — **N/A（路徑未採用）**：hook 系統與 chat-API PTY 路徑於 phases 3+5 一次性移除，無雙寫過渡階段
- [x] 5.3 切換 admin observability 完全靠 ACP event（hook 寫入路徑停用，但 hook handler 仍存在） — hook handler 已隨 phase 5 全部刪除，admin observability 直接 ACP-only
- [x] 5.4 既有測試 T55/T56/MT12 PASS（batch B 2236 QA report）：
  - T55-single：PASS（routing fix 後）；T55-multi 由 10.3 全套 QA 在 multi-mode 環境涵蓋
  - T56：PASS（搜尋/詳情皆通過；ToolEvents=null 是另開 ticket 級 caveat）
  - MT12：PASS（兩次 query 各自獨立 record）

## 6. Phase 5：Hook 系統移除

- [x] 6.1 grep 確認沒有 code path 寫入 `/hook` endpoint（chat-API、Discord、Telegram 都已切 ACP）
- [x] 6.2 刪除：
  - `hook.go`（整個檔案）
  - `server.go` 中 `/hook` 路由註冊與 `hookHandler`
  - `im.go` 的 `HookEvent` struct、`IMAdapter.Notify(HookEvent, string)` 介面
  - `IMManager.notify()` dispatch
  - `UserSessionManager.NotifyHook()` method
- [x] 6.3 刪除 `claude/settings.json`、`claude/merge-settings.js` 整個 `claude/` 目錄（如果只剩 hook 相關檔；skills 子目錄保留則只刪 settings.json + merge-settings.js）
- [x] 6.4 修改 `Dockerfile` 移除 `COPY claude/ /app/perch-claude/` 中與 hook 相關部分（保留 skills）
- [x] 6.5 修改 `entrypoint.sh`：刪除 `merge-settings.js` 呼叫的 block
- [x] 6.6 grep code 確認沒人 import `HookEvent` / 呼叫 `Notify(HookEvent`
- [x] 6.7 移除既有相關 unit test（`hook_test.go`、`hook_routing_test.go` 整個刪除）

## 7. Phase 6：Cleanup

- [x] 7.1 移除 `runtime.go` 中 chat-API 不再使用的 Claude `-p` mode args builder（保留 OpenCode 對應路徑） — `runtime.go` 已重寫成 ACP-only `RunAgent`，無 `-p` 殘留
- [x] 7.2 確認 `PTYManager` 對 IM 的 wiring 全部清乾淨；保留 `s.pty`（web `/ws` 用） — `grep PTYManager im_*.go` 為空；剩餘 PTYManager 引用都在 `server.go`/`user_session.go`/`scheduler.go` 三處 web `/ws` + scheduler fallback 用途
- [x] 7.3 移除 image `/app/perch-claude/settings.json`、`/app/perch-claude/merge-settings.js` 殘留 — `claude/` 只剩 `skills/`，Dockerfile `COPY claude/` 不再帶 settings/merge-settings；entrypoint.sh 無 merge-settings 呼叫
- [x] 7.4 README / CLAUDE.md 更新：
  - 移除 hook env 設定段（README TOC + Discord Hook Reaction 段已刪）
  - 新增「ACP-only 架構」段（README breaking change block + CLAUDE.md `## 0. 架構速記`）
  - 移除 `DISCORD_ACP_ENABLED` env var 表格 row + Discord 整合段內 PTY/ACP 模式分支
- [x] 7.5 release note 標 **Breaking changes**（寫入 `README.md` Breaking changes block）：
  - `/hook` endpoint 移除（自製 webhook 失效）
  - `DISCORD_ACP_ENABLED` 環境變數移除
  - `claude/settings.json`、`claude/merge-settings.js` 不再 inject

## 8. Admin → management 命名 rename（已決）

- [x] 8.1 路徑 rename：`/api/admin/*` → `/api/management/*`、`/ws/admin` → `/ws/management`
- [x] 8.2 中介層 rename：`adminMW` → `managementMW`
- [x] 8.3 Go struct rename：`AdminHub` → `ManagementHub`、`AdminSessionView` → `ManagementSessionView`、`adminEvent` → `managementEvent`
- [x] 8.4 Handler rename：`handleAdminWS` → `handleManagementWS`、`handleAdminHistory(Detail)?` → `handleManagementHistory(Detail)?`
- [x] 8.5 Frontend：`AdminPage` → `ManagementPage`、相關路由與導覽連結
- [x] 8.6 Capabilities rename：`admin-realtime` → `management-realtime`、`admin-history` → `management-history`（從 `openspec/specs/` 移到新名） — 兩個 dir `git mv` + 內文文字 admin/Admin → management/Management
- [x] 8.7 spec.md 對應改名（本 change 的 specs/ 子目錄與內文） — `specs/management-realtime/spec.md` 內 `admin clients` → `management clients`；新增 `specs/management-history/spec.md` 文件 URL rename diff
- [x] 8.8 release note 列入 Breaking change（URL 改變、自製 management UI 客戶端要更新 base path） — 寫入 `README.md` Breaking changes block 第 3 條
- [x] 8.9 既有測試 ID 文字（T55、T56、MT12 等）對齊新名稱（不改 ID 編號） — `tests/test-{auth-modes,settings,auth-login-ui,kb-chat-api,discord-im}.md` 的 `/api/admin/*` `/ws/admin` `/admin/{history,analytics}` 全部 sed 替換成 `/api/management/*` `/ws/management` `/management/{history,analytics}`

## 9. Live 限 multi-user mode（已決）

- [x] 9.1 在 `server.go` 路由註冊處檢查 `PERCH_MODE`：mode 不等於 `"multi"` 時不註冊 `/ws/management`、`/api/management/*` 也視策略決定（建議：history 仍允許 single-user 自查，僅 live 限 multi）
- [x] 9.2 spec `management-realtime` 加 Requirement「Live access requires multi-user mode」
- [x] 9.3 T55 拆兩個情境：
  - **T55-single**：`PERCH_MODE=single` 下 `/ws/management` 升 WS 應失敗（404 或 connection refused）
  - **T55-multi**：`PERCH_MODE=multi` 下行為等同既有 T55（snapshot/added/update/removed）
- [x] 9.4 frontend：single-user mode 下 management 頁面不顯示 Live 分頁（或灰掉）；multi-user mode 顯示

## 10. 文件與測試

- [x] 10.1 文件已在各 phase 中分配（README/CLAUDE.md/specs/tests 全部對齊 ACP-only）
- [x] 10.2 撰寫 `tests/test-acp-tool-events.md`（新測試檔）— 已建立，包含 AT-E01-04 + 共通前置 + 反向驗證 + cleanup 步驟：
  - **AT-E01**：ACP `tool_call_started` → ManagementHub `session_update` event 帶正確 tool name（WS subscriber 驗證）
  - **AT-E02**：ACP `tool_call_completed` → `tool_events` row 補完 `output_json` 與 `ended_at`
  - **AT-E03**：ACP `RunCompleted` → `session_removed` + `query_sessions` row status=done
  - **AT-E04**：ACP `RunFailed` / timeout → status=error，response 含錯誤訊息
- [x] 10.3 全套 QA cycle 跑：MT01-12、T07、T18、T19、T46、T52、T55、T56、TG-A01-05、AT-E01-04，期望 zero FAIL / zero env-fix-by-qa SKIP — round 1 (`tests/test-report-2026-04-30-full-2345-summary.md`) 5 FAIL → round 2 (`...-0024-round2.md`) 1 PASS / 4 FAIL → round 3 (`...-0032-round3.md`) 4 PASS / 0 FAIL，全綠收結
- [x] 10.4 對比舊測試報告（`tests/test-report-2026-04-30-1236-summary.md`）確認無 regression — round 3 sanity check T19 / MT12 / T55-single 仍 PASS；T55 改 split 成 single/multi 兩個情境，皆 PASS

## 11. 結束條件

- [x] 11.1 `grep -rn "hook\|HookEvent\|Notify.*HookEvent\|/hook" --include="*.go"` 結果僅剩文件 / 註解殘餘 — 殘留註解已盡量清理（im.go IMManager doc、main.go LISTEN_ADDR、user_session.go ClaimUUID dangling comment 等）；目前 `*.go` 內僅剩 MT07/T52 PTY 退場註解（仍有效，描述 web `/ws` 行為）
- [x] 11.2 `grep -rn "DISCORD_ACP_ENABLED\|claude -p" --include="*.go" --include="*.sh"` 結果為空 — 僅 `user_session_test.go` 一個歷史註解，已重寫成 ACP-無關描述
- [x] 11.3 `entrypoint.sh` 不再呼叫 `merge-settings.js` — `grep merge-settings entrypoint.sh` 為空
- [x] 11.4 全套 batch B QA cycle 全綠 — round 3 (`tests/test-report-2026-04-30-full-0032-round3.md`) 全綠收結；container-compat batch B 全綠 (`tests/test-report-2026-04-30-batchB-2236.md`)
- [x] 11.5 README、CLAUDE.md 描述與 code 一致 — README 整段重寫 ACP-only / 新 Discord Reaction 表 / Breaking changes block；perch CLAUDE.md 加 `## 0. 架構速記` 段
- [ ] 11.6 兩個 change（`fix-claude-code-container-compat` + `consolidate-acp-runtime`）archive 完成
