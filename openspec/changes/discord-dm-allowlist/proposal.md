## Why

Discord DM 功能目前對任何知道 Bot 用戶 ID 的人開放，任何人都能私訊 Bot 並直接控制 Claude Code，包括非 Server 成員。這是個嚴重的安全問題，應在推出此功能前加以限制。

## What Changes

- DM 功能預設**關閉**（之前預設開啟）
- 新增環境變數 `DISCORD_ALLOWED_USER_IDS`：逗號分隔的 Discord 用戶 ID 白名單
- 只有白名單內的用戶可透過 DM 與 Bot 互動；白名單為空時，所有 DM 一律靜默忽略
- Private Guild channel 的行為不受影響（仍依頻道權限控制）
- Public Guild channel 的 @mention 行為不受影響

## Capabilities

### New Capabilities
- `discord-dm-allowlist`：DM 用戶白名單機制，透過環境變數控制哪些用戶可透過 DM 使用 Bot

### Modified Capabilities
- `discord-open-channels`：DM 路由行為改變——原本所有 DM 直接進 PTY，現在加上白名單檢查

## Impact

- `im_discord.go`：`onMessage` 的 DM 分支加入白名單過濾
- `main.go`：讀取並傳入 `DISCORD_ALLOWED_USER_IDS` 環境變數
- `README.md`：新增 `DISCORD_ALLOWED_USER_IDS` 說明
