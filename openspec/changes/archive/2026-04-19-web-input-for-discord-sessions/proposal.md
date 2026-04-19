## Why

Discord 建立的 PTY session 可以在 Web 上觀看，但目前 Web 端無法輸入文字——`handleSessionWS` 只接受 resize 訊息，其他輸入一律丟棄，且 `SessionProvider` 介面也沒有 write 方法。這讓使用者在 Web 上觀察 Discord session 時無法直接干預，降低了 Web UI 的實用性。

## What Changes

- 在 `SessionProvider` 介面新增 `WriteSession(channelID string, data []byte) error`
- `DiscordSessionManager` 實作 `WriteSession`，將資料寫入對應 PTY
- `handleSessionWS` 解除對非 resize 輸入的丟棄，改為呼叫 `WriteSession` 將 keystrokes 寫入 PTY

## Capabilities

### New Capabilities
- `web-session-input`: Web WebSocket 端點支援雙向輸入，允許使用者從 Web UI 向 Discord session 的 PTY 寫入資料

### Modified Capabilities
（無 spec-level 行為變更）

## Impact

- `im.go`：`SessionProvider` 介面新增 `WriteSession`
- `im_discord.go`：`DiscordSessionManager` 實作 `WriteSession`
- `server.go`：`handleSessionWS` 接收並轉發 keystrokes
- `server_test.go` / `im_discord_test.go`：補充相關測試
