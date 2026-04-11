# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。

---

## 功能

- **完整 terminal**：基於 xterm.js，支援顏色、滾動、可點擊的 URL
- **即時串流**：所有連線共用同一個 PTY session，即時看到 Claude Code 輸出
- **手機支援**：虛擬鍵盤（Tab、Ctrl+C/D/Z、Esc、方向鍵），視窗縮放自動調整
- **三種認證模式**：無認證（內網測試）、密碼登入、mTLS 雙向憑證
- **排程器**：設定每天幾點自動送指令進 terminal
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
  -v perch-data:/app/data \
  ghcr.io/fcwu/perch:latest

# mTLS 模式（最安全，正式對外使用）
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=mtls \
  -v ~/.claude:/root/.claude \
  -v ~/.claude.json:/root/.claude.json \
  -v /your/workspace:/workspace \
  -v perch-data:/app/data \
  ghcr.io/fcwu/perch:latest
```

---

## 環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `AUTH_MODE` | `none` | 認證模式：`none` / `password` / `mtls` |
| `AUTH_PASSWORD` | — | 密碼（`AUTH_MODE=password` 時必填） |
| `LISTEN_ADDR` | `:8443` | 監聽位址，例如 `:8443` 或 `0.0.0.0:443` |
| `BLOCK_IPS` | — | 空格分隔的封鎖 IP 清單，支援 CIDR，例如 `1.2.3.4 10.0.0.0/8` |
| `CLAUDE_WORKDIR` | `/workspace`（若存在） | Claude Code 的起始工作目錄 |
| `ANTHROPIC_API_KEY` | — | Anthropic API 金鑰，直接傳給 Claude（見下方說明） |
| `DISCORD_BOT_TOKEN` | — | Discord bot token（啟用 Discord 整合） |
| `DISCORD_CHANNEL_ID` | — | 要監聽的 Discord channel ID |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token（啟用 Telegram 整合） |
| `TELEGRAM_CHAT_ID` | — | 要監聽的 Telegram chat ID |

### Claude 認證

Claude Code 使用 OAuth 登入。將主機的 `~/.claude` 掛載進容器，Claude 即可直接使用已登入的憑證：

```bash
-v ~/.claude:/root/.claude
```

若 OAuth 憑證無法使用（例如跨平台或憑證過期），可改用 API 金鑰：

```bash
-e ANTHROPIC_API_KEY=your_key_here
```

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

## 在手機上使用

1. 確認手機與電腦 / server 在同一網路（或 server 有公網 IP）
2. 手機 Chrome 開啟：
   - `none` / `password` 模式：`http://<server-ip>:8080`
   - `mtls` 模式：`https://<server-ip>:8443`
3. 畫面下方有虛擬鍵盤，點擊按鈕送出特殊按鍵

虛擬鍵盤按鍵：

| 按鈕 | 送出 |
|------|------|
| Tab | Tab 補全 |
| Ctrl+C | 中斷目前程序 |
| Ctrl+D | EOF / 登出 |
| Ctrl+Z | 暫停程序 |
| Esc | Escape |
| ↑ ↓ ← → | 方向鍵（歷史指令等） |
| ▼ | 收合鍵盤 |

---

## 排程器 API

可以設定每天特定時間自動送指令進 terminal（例如：每天早上 9 點叫 Claude 做 daily review）。

### 列出排程

```bash
curl -s http://localhost:8080/schedule
```

### 新增排程

```bash
curl -s -X POST http://localhost:8080/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "hour": 9,
    "minute": 0,
    "message": "幫我做今天的 daily standup 摘要",
    "repeat": true
  }'
```

`repeat: true` = 每天重複；`repeat: false` = 只執行一次。

### 刪除排程

```bash
curl -s -X DELETE http://localhost:8080/schedule/<id>
```

排程資料存在 `schedules.json`，重啟後不遺失（Docker 需掛 volume）。

---

## Docker Mount 說明

| Mount | 用途 |
|-------|------|
| `-v ~/.claude:/root/.claude` | Claude Code 登入憑證、設定、技能 |
| `-v ~/.claude.json:/root/.claude.json` | Claude Code UI 狀態（主題、onboarding 記錄）；缺少此掛載會每次重跑主題 wizard |
| `-v /your/workspace:/workspace` | Claude Code 工作目錄 |
| `-v perch-data:/app/data` | 排程資料持久化 |

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
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

---

## License

MIT
