## Why

Perch 目前只能透過瀏覽器操作 Claude Code。加入 IM 整合後，使用者可直接從 Discord 或 Telegram 傳訊息給 Claude，並透過 emoji reaction 即時看到執行狀態，不需要開啟瀏覽器。

## What Changes

- 新增 `IMManager`：管理多個 IM adapter 的 goroutine，收到訊息後寫入 PTY
- 新增 `DiscordAdapter`：使用 `bwmarrin/discordgo` 監聽指定 channel，支援 emoji reaction
- 新增 `TelegramAdapter`：使用 `go-telebot/telebot` 監聽指定 chat，支援文字回應
- 新增 `POST /hook` endpoint：接收 Claude Code Hooks 事件（PreToolUse / PostToolUse / Stop），驅動 reaction 與回應
- 新增 Claude Code Hook 設定：bake 進 `/root/.claude/settings.json`，自動連線 `/hook` endpoint
- 更新環境變數：新增 `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`

## Capabilities

### New Capabilities

- `im-receive`: 從 Discord/Telegram 接收訊息並寫入 PTY
- `im-notify`: 將 Claude Hook 事件轉換為 IM 通知（Discord reaction / Telegram 訊息）

### Modified Capabilities

（無現有 spec 需要修改）

## Impact

- 新增 Go 依賴：`github.com/bwmarrin/discordgo`, `gopkg.in/telebot.v3`
- 新增 `claude/settings.json`（hook 設定），bake 進 Docker image 的 `/root/.claude/`
- `main.go`：根據環境變數決定是否啟動 IM adapters
- Dockerfile：新增 `claude/settings.json` 的 COPY
- 不影響現有認證、PTY、排程器、WebSocket 邏輯
