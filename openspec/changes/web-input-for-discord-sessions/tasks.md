## 1. Interface Extension

- [x] 1.1 在 `im.go` 的 `SessionProvider` 介面新增 `WriteSession(channelID string, data []byte) error`

## 2. DiscordSessionManager 實作

- [x] 2.1 在 `im_discord.go` 實作 `DiscordSessionManager.WriteSession`，找到 channelID 對應的 PTY 並寫入 data；找不到時回傳 error

## 3. Server 輸入轉發

- [x] 3.1 修改 `server.go` 的 `handleSessionWS`：接收 WebSocket 訊息時先嘗試 JSON unmarshal 判斷是否為 resize，否則呼叫 `sessions.WriteSession` 轉發 keystrokes
- [x] 3.2 WriteSession 回傳 error 時 log 並繼續（不關閉 WebSocket）

## 4. 測試

- [x] 4.1 在 `im_discord_test.go` 補充 `WriteSession` 的單元測試（session 存在 / 不存在兩種情境）
- [x] 4.2 在 `server_test.go` 補充 WebSocket 輸入轉發測試（keystroke 轉發 + resize 不被轉發）
- [x] 4.3 執行 `go test ./...` 確認全部通過
