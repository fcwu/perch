## 1. main.go - 放寬 Discord 初始化條件

- [x] 1.1 將 Discord 啟動條件從 `discordToken != "" && discordChannel != ""` 改為 `discordToken != ""`
- [x] 1.2 確保 `im` manager 在只有 Discord token 時也會被建立（不依賴 Telegram 條件）

## 2. im_discord.go - Intent 更新

- [x] 2.1 在 `Start()` 的 `session.Identify.Intents` 中加入 `discordgo.IntentsMessageContent`

## 3. im_discord.go - Private Channel Cache

- [x] 3.1 在 `DiscordSessionManager` 加入 `channelPrivate map[string]bool` 欄位，並在 `newDiscordSessionManager` 初始化
- [x] 3.2 實作 `isPrivateChannel(s *discordgo.Session, channelID string) bool`：先查 cache，miss 時呼叫 `s.Channel(channelID)` 檢查 `@everyone` role 的 `PermissionViewChannel` deny bit，結果存入 cache

## 4. im_discord.go - onMessage DM vs Private vs Public Guild 分流

- [x] 4.1 在 `onMessage` 中加入 `isDM := m.GuildID == ""` 和 `isPrivate := !isDM && d.isPrivateChannel(s, m.ChannelID)` 判斷
- [x] 4.2 若不是 DM 且不是 private channel 且 `allowedChannelID` 為空，檢查 `m.Mentions` 是否包含 Bot 自身 ID，不包含則 return
- [x] 4.3 去除 public Guild 訊息中的 mention prefix（`<@BOT_ID>`）再寫入 PTY
- [x] 4.4 若 strip 後 content 為空字串則 return（不寫入 PTY）

## 4. README.md - 文件更新

- [x] 4.1 更新 Discord 設定說明：`DISCORD_CHANNEL_ID` 標為選填
- [x] 4.2 新增 Open-channel 模式說明：需在 Discord Developer Portal 開啟 Message Content Intent
- [x] 4.3 新增 Bot 邀請 URL 範例與所需權限說明（Read Messages, Send Messages, Add Reactions, Read Message History）

## 5. 驗證

- [x] 5.1 編譯確認無錯誤：`go build ./...`
- [x] 5.2 手動測試：public Guild 頻道 @mention 有回應，未 @mention 無回應 *(需實際 Discord 環境)*
- [x] 5.3 手動測試：private Guild 頻道直接回應，不需 @mention *(需實際 Discord 環境)*
- [x] 5.4 手動測試：DM 直接回應，不需 @mention *(需實際 Discord 環境)*
- [x] 5.5 回歸測試：設定 `DISCORD_CHANNEL_ID`，確認舊行為不受影響 *(需實際 Discord 環境)*
