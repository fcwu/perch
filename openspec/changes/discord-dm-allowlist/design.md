## Context

`DiscordSessionManager.onMessage` 目前對 DM（`isDM = m.GuildID == ""`）不做任何授權檢查，直接呼叫 `getOrCreateSession` 並寫入 PTY。這讓任何 Discord 用戶都能控制 Claude Code。

`DISCORD_ALLOWED_USER_IDS` 環境變數尚不存在，需新增解析邏輯並傳入 `DiscordSessionManager`。

## Goals / Non-Goals

**Goals:**
- DM 預設關閉（`DISCORD_ALLOWED_USER_IDS` 未設定 → 所有 DM 靜默忽略）
- 設定白名單後，只有名單內的用戶 ID 可透過 DM 使用 Bot
- Private Guild channel 及 Public Guild @mention 行為不變

**Non-Goals:**
- 不實作 Guild 層級的白名單（Private channel 仍由 Discord 頻道權限控制）
- 不支援用戶名稱或 tag（只接受數字 user ID，避免解析歧義）
- 不提供動態更新白名單的機制（重啟 container 才生效）

## Decisions

### D1：白名單存為 `map[string]struct{}`，不用 slice

Set lookup O(1) vs O(n)，且語意上白名單是集合。啟動時由逗號分隔字串解析一次，後續查詢只做 map lookup。

### D2：白名單為空 → 全拒絕（deny-by-default）

空白名單代表「未設定」，應解讀為「不開放 DM」。若解讀為「全開放」則與現有行為相同，無法修復安全問題。

### D3：白名單只影響 DM，Private channel 不受限

Private channel 本身已由 Discord Server 的頻道權限把關（只有被邀請的用戶可見），不需要額外的 user-level 過濾。

### D4：`allowedDMUserIDs` 作為 `DiscordSessionManager` 欄位傳入

與 `allowedChannelID` 的設計一致：在 `newDiscordSessionManager` 初始化時注入，避免全域狀態。

## Risks / Trade-offs

- **用戶 ID 輸入錯誤** → Bot 靜默忽略 DM，難以 debug。緩解：啟動時 log 白名單長度（不 log 具體 ID）。
- **白名單更新需重啟** → 設計選擇，屬 Non-Goal；接受此限制。

## Migration Plan

1. 部署新版 image
2. 若需開放 DM：在 `docker run` 加上 `-e DISCORD_ALLOWED_USER_IDS=<your_discord_user_id>`
3. 若不設定：現有 DM 行為靜默關閉（Breaking change，但為安全性所必要）
4. Rollback：退回舊版 image（DM 重新對所有人開放）
