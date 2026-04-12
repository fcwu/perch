## Why

目前 Discord 整合需要同時設定 `DISCORD_BOT_TOKEN` 和 `DISCORD_CHANNEL_ID`，Bot 只會回應指定的單一頻道，無法被邀請到任意 Server 後自由使用所有頻道。移除硬寫 channel ID 的限制，讓 Bot 在任何有權限的頻道都能運作。

## What Changes

- `DISCORD_CHANNEL_ID` 改為選填：只要設定 `DISCORD_BOT_TOKEN` 即可啟動 Discord Bot
- 未設定 `DISCORD_CHANNEL_ID` 時，Bot 監聽所有有權限的頻道
- Guild 頻道（Server 內頻道）：需要 @mention Bot 才會回應，避免干擾其他對話
- DM（私訊）：直接對話，不需要 @mention
- 設定 `DISCORD_CHANNEL_ID` 時維持原有行為（向下相容）
- 更新 README：新增無 channel ID 的設定說明與邀請 Bot 的步驟

## Capabilities

### New Capabilities

- `discord-open-channels`: Bot 可在未指定 channel ID 的情況下自由運作於所有頻道，Guild 頻道需 @mention 觸發，DM 直接對話

### Modified Capabilities

<!-- none -->

## Impact

- `main.go`：放寬 Discord 初始化條件，`DISCORD_CHANNEL_ID` 不再強制必填
- `im_discord.go`：`onMessage` 新增 DM vs Guild 頻道判斷邏輯；需加入 `IntentsMessageContent` privileged intent 以讀取 Guild 頻道訊息內容；`Start()` 中 intent 設定更新
- `README.md`：新增 open-channel 設定說明與 OAuth2 邀請 URL 權限說明
