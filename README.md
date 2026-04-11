# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。

---

## 功能

- **完整 terminal**：基於 xterm.js，支援顏色、滾動、可點擊的 URL
- **即時串流**：所有連線共用同一個 PTY session，即時看到 Claude Code 輸出
- **三種認證模式**：無認證（內網測試）、密碼登入、mTLS 雙向憑證
- **排程器**：用自然語言設定每天幾點自動送指令進 terminal（透過 `local-schedule` skill）
- **IP 封鎖**：TCP 層封鎖惡意 IP
- **限速**：HTTP 層限制登入/bootstrap 端點的請求頻率
- **自動重啟**：Claude Code 崩潰後自動重啟

---

## 快速開始

```bash
# 從 GitHub Container Registry 拉取
docker pull ghcr.io/fcwu/perch:latest

# 無認證模式（內網測試）
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/root/.claude \
  -v ~/.claude.json:/root/.claude.json \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest

# 密碼模式
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=password \
  -e AUTH_PASSWORD=你的密碼 \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/root/.claude \
  -v ~/.claude.json:/root/.claude.json \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest

# mTLS 模式（最安全，正式對外使用）
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=mtls \
  -v ~/.claude:/root/.claude \
  -v ~/.claude.json:/root/.claude.json \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

#### Mount 說明

| Mount | 用途 |
|-------|------|
| `-v ~/.claude:/root/.claude` | Claude Code 登入憑證、設定、技能 |
| `-v ~/.claude.json:/root/.claude.json` | Claude Code UI 狀態（主題、onboarding 記錄）；缺少此掛載會每次重跑主題 wizard |
| `-v /your/workspace:/workspace` | Claude Code 工作目錄；排程資料也存於此 |

Perch 的內建 skill（`local-schedule` 等）會在容器啟動時自動合併到掛載的 `~/.claude/skills/` 中，不需要手動複製。

---

## 環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `AUTH_MODE` | `none` | 認證模式：`none` / `password` / `mtls` |
| `AUTH_PASSWORD` | — | 密碼（`AUTH_MODE=password` 時必填） |
| `LISTEN_ADDR` | `:8443` | 監聽位址，例如 `:8443` 或 `0.0.0.0:443` |
| `BLOCK_IPS` | — | 空格分隔的封鎖 IP 清單，支援 CIDR，例如 `1.2.3.4 10.0.0.0/8` |
| `CLAUDE_WORKDIR` | `/workspace`（若存在） | Claude Code 的起始工作目錄 |
| `TZ` | `UTC` | 容器時區，影響排程觸發時間，例如 `Asia/Taipei` |
| `ANTHROPIC_API_KEY` | — | Anthropic API 金鑰，直接傳給 Claude |
| `DISCORD_BOT_TOKEN` | — | Discord bot token（啟用 Discord 整合） |
| `DISCORD_CHANNEL_ID` | — | 要監聽的 Discord channel ID |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token（啟用 Telegram 整合） |
| `TELEGRAM_CHAT_ID` | — | 要監聽的 Telegram chat ID |

---

## 認證模式說明

### `AUTH_MODE=none` — 無認證

無任何驗證，所有人可直接連線。**僅限內網或本地測試使用**，絕對不要暴露在公網。

### `AUTH_MODE=password` — 密碼登入

連線後需輸入密碼才能看到 terminal。密碼以 cookie session 方式儲存。

### `AUTH_MODE=mtls` — 雙向 TLS（mTLS）

最安全的模式，瀏覽器必須安裝 client 憑證才能連線。

**首次設定流程：**

1. 啟動 server（mTLS 模式）
2. 瀏覽器開啟 `https://<your-server>:8443`，**自動跳轉** 到 `/bootstrap` 並下載 `client.p12`
3. 在手機 / 電腦安裝 `client.p12`（密碼：`perch`）
4. Bootstrap 端點自動失效（只能用一次）
5. 之後連線時，瀏覽器自動帶上 client 憑證

**Android Chrome 安裝憑證：**
- 設定 → 安全性 → 加密憑證 → 安裝憑證 → 選擇 `.p12`

**iOS Safari 安裝憑證：**
- 下載後跳出安裝提示 → 去「設定 → 一般 → VPN 與裝置管理」安裝

---

## 排程器

Perch 內建排程功能，可以設定每天特定時間自動送指令進 terminal（例如：每天早上 9 點叫 Claude 做 daily review）。

在 terminal 中直接用自然語言告訴 Claude，例如：

> 「每天早上 9 點幫我做 daily standup 摘要」

Claude 會透過內建的 `local-schedule` skill 設定排程。排程資料存在 workspace 目錄，重啟容器後不遺失。

> 排程時間以容器時區為準，預設 UTC。若需台灣時間，啟動時加上 `-e TZ=Asia/Taipei`。

---

## IM 整合（Discord / Telegram）

訊息從 Discord / Telegram 進來，寫進 PTY，Claude 處理完後透過 Claude Code Hooks 把結果送回 IM。

### Hook 與 Reaction 對應

| Claude Hook 事件 | Discord | Telegram |
|------------------|---------|----------|
| 收到訊息（進入 PTY）| 👀 | — |
| `PreToolUse` | ⚙️ | — |
| `PostToolUse` 成功 | ✅ | — |
| `PostToolUse` 失敗 | ❌ | — |
| `Stop`（回應完成）| 💬 + 文字訊息 | 文字訊息 |
| 回應超過 2000 字 | 📎 附件 | 文件 |

---

## Discord Bot 設定

### 步驟一：建立 Bot

1. 前往 [Discord Developer Portal](https://discord.com/developers/applications)
2. 點 **New Application** → 輸入名稱（例如 `perch`）→ Create
3. 左側選 **Bot** → 點 **Add Bot**
4. 在 **TOKEN** 區塊點 **Reset Token** → 複製 token → 存為 `DISCORD_BOT_TOKEN`
5. 在同一頁往下找 **Privileged Gateway Intents**，開啟 **Message Content Intent**（必填，否則收不到訊息內容）

### 步驟二：邀請 Bot 進 Server

1. 左側選 **OAuth2 → URL Generator**
2. Scopes 勾選：`bot`
3. Bot Permissions 勾選：
   - `Read Messages / View Channels`
   - `Send Messages`
   - `Add Reactions`
   - `Read Message History`
4. 複製產生的 URL → 在瀏覽器開啟 → 選擇要加入的 Server → Authorize

### 步驟三：取得 Channel ID

1. Discord 開啟 **User Settings → Advanced** → 啟用 **Developer Mode**
2. 右鍵點擊要監聽的 channel → **Copy Channel ID** → 存為 `DISCORD_CHANNEL_ID`

### 步驟四：啟動

```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -e DISCORD_BOT_TOKEN=your_bot_token \
  -e DISCORD_CHANNEL_ID=your_channel_id \
  -v ~/.claude:/root/.claude \
  -v ~/.claude.json:/root/.claude.json \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

---

## License

MIT
