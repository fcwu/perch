## 1. 依賴與介面定義

- [x] 1.1 新增 go.mod 依賴：`github.com/bwmarrin/discordgo` 和 `gopkg.in/telebot.v3`
- [x] 1.2 定義 `IMAdapter` interface（`Start`, `Stop`, `Notify`）和 `HookEvent` struct
- [x] 1.3 定義 `IMManager` struct，持有 `[]IMAdapter` 和 `lastMsg map[string]PendingMessage`

## 2. Hook Endpoint

- [x] 2.1 新增 `hook.go`：實作 `POST /hook` handler，解析 Claude Hook JSON payload
- [x] 2.2 在 `server.go` 註冊 `/hook` route，傳入 `IMManager`
- [x] 2.3 撰寫 `hook_test.go`：測試 valid / invalid payload、各 event type

## 3. Discord Adapter

- [x] 3.1 新增 `im_discord.go`：實作 `DiscordAdapter`（使用 `bwmarrin/discordgo`）
- [x] 3.2 實作收訊息：監聽 `DISCORD_CHANNEL_ID`，過濾其他 channel，寫入 PTY，記錄 `lastMsg`，加 👀 reaction
- [x] 3.3 實作 `Notify`：依 event type 加 reaction（⚙️ / ✅ / ❌ / 💬），Stop 時送 reply 並清除 `lastMsg`
- [x] 3.4 Reaction 失敗時 log warning，不 crash

## 4. Telegram Adapter

- [x] 4.1 新增 `im_telegram.go`：實作 `TelegramAdapter`（使用 `gopkg.in/telebot.v3`）
- [x] 4.2 實作收訊息：過濾非 `TELEGRAM_CHAT_ID` 的訊息，寫入 PTY，記錄 `lastMsg`
- [x] 4.3 實作 `Notify`：Stop 事件時送文字 reply，清除 `lastMsg`

## 5. 主程式整合

- [x] 5.1 更新 `main.go`：讀取 IM 環境變數，條件啟動 `IMManager`
- [x] 5.2 `IMManager.Start()` 啟動所有 adapter goroutine；`Stop()` 優雅關閉
- [x] 5.3 確認沒有 token 時 Perch 正常啟動，無錯誤日誌

## 6. Hook 設定 Bake 進 Image

- [x] 6.1 新增 `claude/settings.json`，設定 PreToolUse / PostToolUse / Stop hooks（curl 呼叫 `/hook`）
- [x] 6.2 確認 `LISTEN_ADDR` 在 settings.json 中以 shell expansion 正確展開，或改用固定 localhost path
- [x] 6.3 驗證 Dockerfile 已 `COPY claude/ /root/.claude/`（已完成，確認 settings.json 包含在內）

## 7. 文件與測試

- [x] 7.1 更新 `docs/test-cases.md`：新增 T18（Discord 收訊息）、T19（Hook reaction）
- [x] 7.2 驗證 README.md 的 Discord Bot 設定步驟與實際實作一致
- [x] 7.3 實測：Stop hook 的 JSON payload 確認是否包含 Claude 回應文字，決定回應策略
