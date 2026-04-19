## 1. main.go — 解析環境變數

- [x] 1.1 讀取 `DISCORD_ALLOWED_USER_IDS`，以逗號分隔並 trim 空白，轉為 `[]string`
- [x] 1.2 將解析後的 slice 傳入 `newDiscordSessionManager`

## 2. im_discord.go — DiscordSessionManager 結構

- [x] 2.1 在 `DiscordSessionManager` 加入 `allowedDMUserIDs map[string]struct{}` 欄位
- [x] 2.2 更新 `newDiscordSessionManager` 簽名接受 `allowedDMUserIDs []string` 並初始化 map

## 3. im_discord.go — onMessage DM 過濾

- [x] 3.1 在 `onMessage` 的 `isDM` 分支加入白名單檢查：若 `m.Author.ID` 不在 `allowedDMUserIDs` 則 return
- [x] 3.2 白名單為空（map length == 0）時，所有 DM 一律 return（deny-by-default）

## 4. README.md — 文件更新

- [x] 4.1 在環境變數表格新增 `DISCORD_ALLOWED_USER_IDS`：選填，逗號分隔的 Discord 用戶 ID 白名單；未設定時 DM 功能關閉
- [x] 4.2 在 Discord 整合說明補充安全提示：DM 預設關閉，如需開啟需明確設定白名單

## 5. 驗證

- [x] 5.1 編譯確認無錯誤：`go build ./...`
- [ ] 5.2 手動測試：未設定 `DISCORD_ALLOWED_USER_IDS`，DM Bot 無回應 *(需實際 Discord 環境)*
- [ ] 5.3 手動測試：設定自己的用戶 ID，DM Bot 正常回應 *(需實際 Discord 環境)*
- [ ] 5.4 手動測試：設定其他用戶 ID（不含自己），DM 無回應 *(需實際 Discord 環境)*
