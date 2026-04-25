# Discord IM 整合 測試案例

> 功能：discord-im
> 涵蓋範圍：單一 Channel 模式、Open-Channel 模式（per-channel PTY）、排程觸發回傳、Discord DM、@mention 過濾、backward compat、ACP 模式。
> 相關 openspec：`im-integration`、`discord-dynamic-channel-support`、`discord-dm-allowlist`、`discord-acp`。
> 撰寫日期：2026-04-20

---

## Discord 頻道對照表

| 頻道 | 類型 | Channel ID |
|------|------|-----------|
| #general | public | `1496275503961608287` |
| #myprivate | private | `1496278101166915694` |
| #myprivate2 | private（設定頻道） | `1496644257149353994` |
| DM with perch-dev | DM | `1492891645413167194` |
| Server ID | — | `1496275499482091611` |

> **傳訊自動化**：所有需要「使用者在 Discord 傳訊」的步驟，均可用 `chrome-cdp` 導航到對應 channel URL，點擊輸入框 `[data-slate-editor]`，`type` 訊息後送出 Enter key event，不需真人操作。

---

## T18 — Discord 訊息寫入 PTY

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.acp_enabled` 設為 `false`，再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.acp_enabled` 設回 `true`，再重啟並等待 server 回來。

**Given** Perch 已完成 Discord Bot 設定並以 PTY 模式啟動（`DISCORD_ACP_ENABLED=false`）
**When** 使用者在指定 Discord channel 傳送訊息：「你好，今天幾號？」
**Then**
- 訊息上出現 👀 reaction，表示 Bot 已收到
- 瀏覽器 terminal 可看到該訊息的文字出現在畫面中

---

## T19 — Discord Hook Reaction 狀態機

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.acp_enabled` 設為 `false`，再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.acp_enabled` 設回 `true`，再重啟並等待 server 回來。

**Given** Discord bot 已連線，Claude Code Hooks 已啟用，Perch 以 PTY 模式啟動（`DISCORD_ACP_ENABLED=false`）
**When** 使用者在 Discord 傳送會觸發工具的指令，例如：「列出 /workspace 下的所有檔案」
**Then** 訊息上的 reaction 依序變化：
- 傳送後 → 👀 出現（已接收）
- Claude 使用工具時 → ⚙️ 出現（執行中）
- 工具完成後 → ✅ 出現，⚙️ 消失
- Claude 回應完畢 → 💬 出現，👀 消失，Discord 收到 reply 訊息

---

## T28 — Discord Session Web Viewer（分頁顯示）

> **適用模式**：僅 PTY 模式（`DISCORD_ACP_ENABLED` 未設或為 false）。ACP 模式下無 terminal tab，此測試**不適用**。

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.acp_enabled` 設為 `false`，再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.acp_enabled` 設回 `true`，再重啟並等待 server 回來。

**Given** Perch 以 Discord Bot 設定啟動（PTY 模式，未設 `DISCORD_ACP_ENABLED`）
**When** 使用者在瀏覽器開啟 Perch，並點擊頁面上方 tab 列中的 Discord channel tab
**Then**
- terminal 畫面切換為該 Discord channel 的輸出
- 從 Discord 傳送訊息後，web viewer 可即時看到 Claude 的回應
- Discord tab 支援鍵盤輸入，輸入的文字會直接寫入對應的 session

**反向驗證**：未設定 Discord 環境變數時，tab 列不顯示（只有主 terminal）。

---

## T29 — Discord Session PTY Resize

> **適用模式**：僅 PTY 模式。ACP 模式下沒有 terminal tab，此測試**不適用**。

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.acp_enabled` 設為 `false`，再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.acp_enabled` 設回 `true`，再重啟並等待 server 回來。

**Given** 使用者正在瀏覽器中檢視 Discord channel tab（PTY 模式）
**When** 使用者調整瀏覽器視窗大小，然後在 Discord 傳送一個指令
**Then**
- terminal 畫面填滿調整後的視窗，無空白邊緣
- 指令的輸出換行位置正確，符合新的視窗寬度

---

## T31 — Discord 排程觸發回傳到正確 Channel

**層級**：E2E-browser（含 Discord 整合）

**Given** 使用者透過 Discord 告訴 Claude 建立一個一次性排程，目標為當前 channel，觸發時間設為 1 分鐘後
**When** 排程時間到，觸發執行
**Then**
- Discord channel 出現排程 header 訊息（格式：`📅 local schedule > 訊息內容`）
- Claude 完成後，回覆出現在同一 channel，附在 header 下方
- 主 terminal 沒有任何新輸出（訊息未流入主 PTY）
- 這個一次性排程在觸發後自動移除

**反向驗證**：若排程的 `target` 欄位為空，訊息應出現在主 terminal，Discord 無反應。

---

## T32 — Discord 排程 Header 訊息格式

**層級**：E2E-browser（含 Discord 整合）

**Given** 一個以 Discord channel 為目標的排程剛被觸發（如 T31）
**When** 使用者觀察 Discord channel 的訊息
**Then**
- 先看到 header 訊息，格式為：`📅 local schedule > {排程訊息內容}`
- Claude 的回覆以 Discord reply 形式附在 header 下方，可清楚辨識是排程觸發的結果

---

## T34 — Discord Open-Channel 模式：僅設 BOT_TOKEN 即可啟動

**層級**：E2E-browser

**Given** Perch 只設定 `DISCORD_BOT_TOKEN`，不設 `DISCORD_CHANNEL_ID`
**When** container 啟動
**Then**
- 啟動成功，log 顯示 Discord bot 已以 per-channel 模式連線
- 不出現「DISCORD_CHANNEL_ID 必填」或其他啟動失敗訊息
- 瀏覽器可正常開啟 terminal

---

## T35 — Discord Open-Channel：Public 頻道需 @mention

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 清空 `discord.channel_id`（設為空字串），再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.channel_id` 還原為原設定值，再重啟並等待 server 回來。

**Given** Perch 以純 `DISCORD_BOT_TOKEN`（不帶 `DISCORD_CHANNEL_ID`）啟動，測試頻道為 public（#general）
**When** 使用者在 public 頻道直接傳送訊息（不 @mention Bot）：「你好」
**Then** 30 秒內無任何 reaction 或回應，Bot 靜默忽略

**When** 使用者在同一頻道傳送 @mention 訊息：`@Perch 你好`
**Then** 訊息出現 👀 reaction；Claude 處理完後 Discord 收到 reply

---

## T36 — Discord Open-Channel：Private 頻道直接回應（不需 @mention）

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 清空 `discord.channel_id`（設為空字串），再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.channel_id` 還原為原設定值，再重啟並等待 server 回來。

**Given** Perch 以純 `DISCORD_BOT_TOKEN` 啟動，測試頻道為 private（#myprivate）
**When** 使用者在 private 頻道直接傳送訊息（不 @mention）：「你是誰？」
**Then**
- 訊息出現 👀 reaction
- Claude 處理後 Discord 收到 reply，內容回答問題

---

## T37 — Discord Open-Channel：DM 直接回應（不需 @mention）

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 清空 `discord.channel_id`（設為空字串），再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.channel_id` 還原為原設定值，再重啟並等待 server 回來。

**Given** Perch 以純 `DISCORD_BOT_TOKEN` 啟動（無 `DISCORD_CHANNEL_ID`），Discord user ID 為 `1075643998632419380` 的使用者開啟與 Bot 的私訊（DM）
**When** 使用者直接傳送：「今天日期是？」
**Then**
- 訊息出現 👀 reaction
- Claude 回應後，Bot 在 DM 中回覆正確日期
- Web terminal 出現對應的 Discord DM session tab

---

## T38 — Discord Backward Compat：設定 DISCORD_CHANNEL_ID 維持原行為

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.channel_id` 設為 `1496278101166915694`（#myprivate），再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.channel_id` 還原為 `1496644257149353994`，再重啟並等待 server 回來。

**Given** Perch 同時設定 `DISCORD_BOT_TOKEN` 與 `DISCORD_CHANNEL_ID=1496278101166915694`（指向 #myprivate）
**When** 使用者在指定頻道（#myprivate）傳送訊息（不需 @mention）：「你好」
**Then** 訊息出現 👀 reaction，Claude 正常回應（維持原有行為）

**When** 使用者在其他頻道（#myprivate2）傳送相同訊息
**Then** 無任何 reaction 或回應（channel filter 生效）

---

## T39 — Discord mention prefix 剝除（Public 頻道）

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 清空 `discord.channel_id`（設為空字串），再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.channel_id` 還原為原設定值，再重啟並等待 server 回來。

**Given** Perch 以純 `DISCORD_BOT_TOKEN` 啟動（無 channel filter），測試頻道為 public（#general）
**When** 使用者傳送：`@Perch 列出 /workspace 下的檔案`
**Then**
- Claude 收到的問題只有「列出 /workspace 下的檔案」，沒有 `<@BOT_ID>` 前綴
- Claude 正常執行對應指令並回覆結果

---

## T40 — ACP 模式啟動確認

**層級**：E2E-curl

**Given** 環境變數設定如下：
```
DISCORD_BOT_TOKEN=<token>
DISCORD_ACP_ENABLED=true
ACP_EXECUTABLE=claude-agent-acp
ACP_RUN_TIMEOUT=120
LISTEN_ADDR=:18080
```
**When** 啟動 Perch binary
**Then**
- 啟動 log 中出現 ACP 模式已啟用的訊息（包含 `acp` 或 `ACP` 關鍵字），而非 PTY 模式訊息
- `curl http://localhost:18080/` 回傳 HTTP 200，服務正常運作
- 啟動後瀏覽器 tab 列不出現 Discord channel terminal tab（ACP 模式無 web terminal）

**反向驗證**：移除 `DISCORD_ACP_ENABLED` 後重啟，log 顯示 PTY 模式，瀏覽器 tab 列出現 Discord channel tab。

---

## T41 — ACP 模式基本問答（👀 → 💬 → reply）

**層級**：E2E-browser（含 Discord 整合）

**Given** Perch 以 ACP 模式啟動（`DISCORD_ACP_ENABLED=true`），Discord bot 已上線
**When** 使用者在 Discord channel 傳送訊息：「今天幾號？」
**Then**
- 訊息傳送後數秒內，訊息上出現 👀 reaction（Bot 已收到，正在處理）
- Claude 完成回應後：
  - 👀 reaction 消失
  - 訊息上出現 💬 reaction
  - Discord channel 出現 Bot 的 reply，內容包含正確日期
- 回應期間 Perch web terminal 不出現任何新輸出（ACP 模式不寫入 PTY）

---

## T42 — ACP 模式：Web Terminal Tab 不存在

**層級**：E2E-browser

**Given** Perch 以 ACP 模式啟動（`DISCORD_ACP_ENABLED=true`）
**When** 使用者在瀏覽器開啟 Perch 首頁，查看頁面上方的 tab 列
**Then**
- tab 列中只顯示主 terminal tab
- 沒有任何 Discord channel 對應的 terminal tab
- 即使在 Discord 傳送訊息並收到回覆後，重新整理頁面仍無 channel tab

---

## T43 — ACP Subprocess Crash Recovery

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：確認 `DISCORD_ACP_ENABLED=true`（預設模式）。先在 Discord 發送一則訊息確認 ACP subprocess 已啟動，再手動 kill process 後測試。

**Given** Perch 以 ACP 模式啟動（`DISCORD_ACP_ENABLED=true`），使用者已在 Discord channel 成功完成過一次問答（subprocess 曾正常執行）
**When** 使用者（或管理員）在 server 端手動 kill 對應 channel 的 `claude-agent-acp` process，然後在 Discord 傳送新訊息：「你還在嗎？」
**Then**
- 訊息上出現 👀 reaction（Bot 正在處理）
- Perch 自動重啟該 channel 的 subprocess
- Claude 正常回應，Discord 收到 reply，訊息上出現 💬 reaction
- 不需要重啟 Perch，crash recovery 全自動完成

---

## T44 — ACP Timeout 行為（❌ + 錯誤訊息）

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：需先設定短 timeout 並重啟容器（加入 `ACP_RUN_TIMEOUT=8`），測試完畢後還原為 `ACP_RUN_TIMEOUT=120`。

**Given** Perch 以 ACP 模式啟動（`DISCORD_ACP_ENABLED=true`），`ACP_RUN_TIMEOUT=8`（8 秒 timeout）
**When** 使用者在 Discord 傳送一個會讓 Claude 長時間處理的任務，使其超過 timeout 上限
**Then**
- 訊息上的 👀 reaction 在 timeout 後消失
- 訊息上出現 ❌ reaction
- Discord channel 收到一則錯誤訊息，說明處理逾時或失敗
- 不出現無限等待（Bot 沒有卡住）

---

## T45 — ACP 模式：多 Channel 各自獨立的 Subprocess

**層級**：E2E-browser（含 Discord 整合）

**Given** Perch 以 ACP 模式啟動，已有兩個不同的 Discord channel（channel-A #myprivate2、channel-B #myprivate）各自傳送過訊息
**When** 使用者在 channel-A 詢問：「你記得我剛才說什麼嗎？」（前一則訊息是「apple」），同時在 channel-B 詢問同樣問題（前一則訊息是「banana」）
**Then**
- channel-A 的 Claude 回應提及「apple」，反映該 channel 的對話脈絡
- channel-B 的 Claude 回應提及「banana」，反映該 channel 的對話脈絡
- 兩個 channel 的回應互不干擾（subprocess 各自獨立）

---

## T46 — PTY Fallback（未設 DISCORD_ACP_ENABLED 維持原行為）

**層級**：E2E-browser（含 Discord 整合）

**前置操作**：透過 `PATCH /api/settings` 將 `discord.acp_enabled` 設為 `false`，再 `POST /api/admin/restart` 重啟並等待 server 回來。**後置操作**：`PATCH /api/settings` 將 `discord.acp_enabled` 設回 `true`，再重啟並等待 server 回來。

**Given** Perch 啟動時設定 `DISCORD_BOT_TOKEN`，且 `DISCORD_ACP_ENABLED=false`（PTY 模式）
**When** 使用者在 Discord channel 傳送訊息，並在瀏覽器開啟 Perch 首頁
**Then**
- Discord channel 訊息出現 👀 reaction，Claude 正常回應
- 瀏覽器 tab 列出現對應的 Discord channel terminal tab
- terminal tab 可看到 Claude 的輸出（PTY 模式行為維持不變）
- 與 ACP 模式下的行為（無 terminal tab）明顯不同
