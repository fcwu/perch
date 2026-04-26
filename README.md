# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。


<!-- @import "[TOC]" {cmd="toc" depthFrom=2 depthTo=6 orderedList=false} -->

<!-- code_chunk_output -->

- [功能](#功能)
  - [Web Terminal 排程](#web-terminal-排程)
  - [Web Terminal 監看 Discord](#web-terminal-監看-discord)
  - [Discord 排程觸發](#discord-排程觸發)
- [快速開始](#快速開始)
  - [Mount 說明](#mount-說明)
- [Runtime Settings](#runtime-settings)
- [環境變數](#環境變數)
  - [必要 / 啟動時設定](#必要--啟動時設定)
  - [GitLab OAuth（多使用者或 GitLab 認證）](#gitlab-oauth多使用者或-gitlab-認證)
  - [Workspace Git Sync](#workspace-git-sync)
  - [可在 Settings UI 調整（env var 為初始值）](#可在-settings-ui-調整env-var-為初始值)
- [推薦使用情境](#推薦使用情境)
  - [情境一：個人知識庫（家用，推薦）](#情境一個人知識庫家用推薦)
  - [情境二：多人知識庫（其他人只能查詢）](#情境二多人知識庫其他人只能查詢)
- [認證方式](#認證方式)
  - [`none` — 無認證](#none--無認證)
  - [`password` — 密碼登入](#password--密碼登入)
  - [`mtls` — 雙向 TLS（最安全）](#mtls--雙向-tls最安全)
  - [`gitlab` — GitLab OAuth（單使用者）](#gitlab--gitlab-oauth單使用者)
- [運作模式](#運作模式)
  - [單使用者模式（預設）](#單使用者模式預設)
  - [多使用者模式](#多使用者模式)
- [Chat UI](#chat-ui)
- [Agent Runtime](#agent-runtime)
- [排程器](#排程器)
- [Discord 整合](#discord-整合)
  - [ACP 模式（推薦）](#acp-模式推薦)
  - [PTY 模式（預設）](#pty-模式預設)
  - [Hook 與 Reaction 對應](#hook-與-reaction-對應)
  - [Discord Bot 設定](#discord-bot-設定)
- [Workspace Git Sync](#workspace-git-sync-1)
- [License](#license)

<!-- /code_chunk_output -->




## 功能

- **完整 terminal**：基於 xterm.js，支援顏色、滾動、可點擊的 URL
- **即時串流**：所有連線共用同一個 PTY session，即時看到 Claude Code 輸出
- **兩種運作模式**：單使用者（`PERCH_MODE=single`）直接存取 terminal；多使用者（`PERCH_MODE=multi`）透過 GitLab OAuth 管理多位使用者，管理員與一般使用者分流
- **四種認證方式**：無認證（內網測試）、密碼登入、mTLS 雙向憑證、GitLab OAuth
- **多輪對話 Chat UI**：網頁聊天介面支援連續追問，對話歷史自動從 SQLite 重建，24 小時內的對話可跨 session 保留
- **Runtime Settings**：認證、限速、agent 參數等可在 UI 熱改，不需重啟容器
- **排程器**：用自然語言設定每天幾點自動送指令進 terminal（透過 `local-schedule` skill）
- **IP 封鎖**：TCP 層封鎖惡意 IP
- **限速**：HTTP 層限制登入/bootstrap 端點的請求頻率
- **自動重啟**：Claude Code 崩潰後自動重啟

### Web Terminal 排程

用自然語言告訴 Claude 設排程，直接生效。排程以 JSONL 存在 workspace，容器重啟不遺失。

![Web Terminal 排程設定](docs/images/schedule-setup.png)

### Web Terminal 監看 Discord

瀏覽器內切換 tab，即時觀看 Discord channel 的 Claude PTY 輸出。

![Web Terminal 監看 Discord](docs/images/discord-tab.png)

### Discord 排程觸發

排程在指定時間自動觸發，Discord channel 先出現來源提示，Claude 回覆以 thread 形式附在下方。

![Discord 排程回覆](docs/images/discord-reply.png)

## 快速開始

```bash
docker run -d \
  -p 127.0.0.1:8080:8080 \
  -e PUID=$(id -u) -e PGID=$(id -g) -e TZ=Asia/Taipei \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  -v ./data:/data \
  ghcr.io/fcwu/perch:latest
```

瀏覽器開啟 `http://localhost:8080`，點右上角齒輪圖示進入 **Settings** 完成後續設定。

### Mount 說明

| Mount | 用途 |
|-------|------|
| `-v ~/.claude:/home/perchuser/.claude` | Claude Code 設定、技能；含 `.credentials.json`（OAuth token） |
| `-v ~/.claude.json:/home/perchuser/.claude.json` | 記錄 `hasCompletedOnboarding` 與 `userID`；缺少時 Claude Code 視為全新安裝，即使憑證存在也會要求重新登入 |
| `-v ./:/workspace` | Claude Code 工作目錄（當前目錄）；排程資料也存於此 |
| `-v ./data:/data` | 持久化 Settings 與對話歷史（`settings.json`、`perch.db`） |

---

## Runtime Settings

啟動後到任意頁面點右上角 **⚙ Settings** 即可修改執行期設定，**不需重啟容器**。

| 分類 | 可調整項目 | 生效時機 |
|------|-----------|---------|
| Auth | 認證方式、密碼 | 重啟後 |
| Agent | CLI 參數（`--dangerously-skip-permissions` 等） | 下次 session |
| Rate Limit | 每分鐘最多請求數（RPM） | 立即 |
| Network | IP 封鎖清單 | 立即 |
| Discord | Bot Token、Channel ID、允許的 User IDs | 重啟後 |
| Telegram | Bot Token、Chat ID | 重啟後 |
| Advanced | Log 格式（text / json） | 下次 session |

Settings 儲存在 `/data/settings.json`。需要重啟的設定，按 **Save & Restart** 即可由 Perch 自動完成。

> **env var 作為初始值**：啟動時設定的 env var 會成為 Settings 的預設值，UI 修改後 `settings.json` 會覆蓋 env var。

---

## 環境變數

只有**容器啟動時**才能設定的項目才需要在 `docker run` 傳入。其餘項目建議啟動後在 Settings UI 調整。

### 必要 / 啟動時設定

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `PUID` | `1000` | 容器內行程的 UID；建議設為 `$(id -u)` |
| `PGID` | `PUID` 同值 | 容器內行程的 GID；建議設為 `$(id -g)` |
| `TZ` | `UTC` | 容器時區，影響排程觸發時間，例如 `Asia/Taipei` |
| `LISTEN_ADDR` | `:8080` | 監聽位址；mTLS 模式需改為 `:8443` |
| `PERCH_MODE` | `single` | `single`（單使用者）/ `multi`（多使用者） |
| `AGENT_RUNTIME` | `claude` | `claude` / `opencode` |
| `CLAUDE_WORKDIR` | `/workspace`（若存在） | Claude Code 的起始工作目錄 |

### GitLab OAuth（多使用者或 GitLab 認證）

| 變數 | 說明 |
|------|------|
| `GITLAB_URL` | GitLab instance URL，例如 `https://gitlab.example.com` |
| `GITLAB_CLIENT_ID` | GitLab OAuth Application 的 Client ID |
| `GITLAB_CLIENT_SECRET` | GitLab OAuth Application 的 Client Secret |
| `GITLAB_REDIRECT_URI` | OAuth callback URI，例如 `https://perch.example.com/auth/callback` |
| `GITLAB_ADMIN_IDS` | 逗號分隔的 GitLab 使用者 ID（管理員） |
| `GITLAB_ALLOWED_IDS` | 逗號分隔的允許使用者 ID；`*` 允許所有已認證使用者 |
| `COOKIE_SECRET` | 簽署 session cookie 的 HMAC 密鑰；**正式環境請務必設定隨機值** |

### Workspace Git Sync

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `WORKSPACE_GIT_SYNC_ENABLED` | `false` | 設為 `true` 啟用自動 git sync |
| `WORKSPACE_GIT_SYNC_INTERVAL` | `60` | Sync 間隔秒數 |
| `WORKSPACE_PATH` | `/workspace` | 要同步的 git repo 路徑 |
| `WORKSPACE_GIT_TOKEN` | — | HTTPS remote 的 git token |
| `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` | — | 同步失敗時送通知的 Discord channel ID |
| `WORKSPACE_GIT_SYNC_SUBMODULES` | `false` | 設為 `true` 在 pull 後自動更新 submodule |

### 可在 Settings UI 調整（env var 為初始值）

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `AUTH_METHOD` | `none` | `none` / `password` / `mtls` / `gitlab` |
| `PERCH_PASSWORD` | — | 密碼（`AUTH_METHOD=password` 時） |
| `ADMIN_TOKEN` | — | Admin 介面 token；設定後開啟 admin 路由保護 |
| `CLAUDE_ARGS` / `OPENCODE_ARGS` | — | 傳給 agent 的額外 CLI 參數 |
| `RATE_LIMIT_RPM` | `10` | 每位使用者每分鐘最多查詢次數；`0` 停用 |
| `LOG_FORMAT` | `text` | `text` 或 `json` |
| `BLOCK_IPS` | — | 空格分隔的封鎖 IP 清單，支援 CIDR |
| `DISCORD_BOT_TOKEN` | — | Discord bot token |
| `DISCORD_CHANNEL_ID` | — | 限制只監聽指定 channel（不設則監聽所有） |
| `DISCORD_ALLOWED_USER_IDS` | — | DM 白名單；**未設定時 DM 功能完全關閉** |
| `DISCORD_ACP_ENABLED` | — | `true` 啟用 ACP stdio 模式 |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | Telegram chat ID |

---

## 推薦使用情境

### 情境一：個人知識庫（家用，推薦）

Perch 本身不做認證（`AUTH_METHOD=none`），存取控制交給 **Cloudflare Zero Trust** 處理。搭配 **Discord Bot** 讓你不用開瀏覽器也能隨時查詢。

```bash
docker run -d \
  -p 127.0.0.1:8080:8080 \
  -e PUID=$(id -u) -e PGID=$(id -g) -e TZ=Asia/Taipei \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  -v ./data:/data \
  ghcr.io/fcwu/perch:latest
```

啟動後在 Settings 設定 Discord Bot Token 即可，不需重啟。

**Cloudflare Zero Trust 設定步驟：**
1. [Cloudflare Zero Trust](https://one.dash.cloudflare.com/) → Networks → Tunnels → 建立 Tunnel，指向 `http://localhost:8080`
2. Access → Applications → 新增 Self-hosted App，設定允許的身份來源（Google、GitHub 等）

### 情境二：多人知識庫（其他人只能查詢）

| 角色 | 設定方式 | 可存取路由 |
|------|---------|-----------|
| 管理員（你） | GitLab ID 加入 `GITLAB_ADMIN_IDS` | `/terminal`（terminal + 管理面板） |
| 團隊成員 | GitLab 登入即可（`GITLAB_ALLOWED_IDS=*`） | `/chat`（只能聊天查詢） |

```bash
docker run -d \
  -p 8080:8080 \
  -e PERCH_MODE=multi \
  -e GITLAB_URL=https://gitlab.com \
  -e GITLAB_CLIENT_ID=<你的 App ID> \
  -e GITLAB_CLIENT_SECRET=<你的 App Secret> \
  -e GITLAB_REDIRECT_URI=https://your-domain/auth/callback \
  -e GITLAB_ADMIN_IDS=123456 \
  -e GITLAB_ALLOWED_IDS=* \
  -e COOKIE_SECRET=$(openssl rand -hex 32) \
  -e PUID=$(id -u) -e PGID=$(id -g) -e TZ=Asia/Taipei \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  -v ./data:/data \
  ghcr.io/fcwu/perch:latest
```

## 認證方式

在 **Settings → Auth Method** 切換，或啟動時用 `AUTH_METHOD` 指定初始值。

### `none` — 無認證

無任何驗證，所有人可直接連線。**僅限內網或本地測試使用**，建議搭配 Cloudflare Zero Trust。

### `password` — 密碼登入

連線後需輸入密碼。密碼在 Settings → Auth → Password 設定，無需重啟容器。

### `mtls` — 雙向 TLS（最安全）

瀏覽器必須安裝 client 憑證才能連線。

**首次設定流程：**
1. 在 Settings 切換到 `mtls` 並重啟
2. 瀏覽器開啟 `https://<your-server>:8443`，**自動跳轉** 到 `/bootstrap` 並下載 `client.p12`
3. 安裝 `client.p12`（密碼：`perch`）— Android: 設定 → 安全性 → 加密憑證；iOS: 設定 → 一般 → VPN 與裝置管理
4. Bootstrap 端點自動失效（只能用一次）

### `gitlab` — GitLab OAuth（單使用者）

使用 GitLab OAuth 認證。需在啟動時設定 `GITLAB_URL`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`。

## 運作模式

### 單使用者模式（預設）

`PERCH_MODE=single`（或不設定）。`/` 自動跳轉到 `/chat`；`/terminal` 給管理員使用。

### 多使用者模式

`PERCH_MODE=multi`，需搭配 GitLab OAuth。

- **管理員**（`GITLAB_ADMIN_IDS`）：登入後路由到 `/terminal`（terminal + 管理面板），也可直接瀏覽 `/chat`
- **一般使用者**（`GITLAB_ALLOWED_IDS`）：登入後路由到 `/chat`
- **未認證的訪客**：顯示 GitLab 登入畫面

## Chat UI

Chat UI 提供多輪對話支援，使用者可以追問後續問題，agent 會自動記住過去 24 小時內的對話歷史。

- **多輪對話**：對話歷史從 SQLite 自動重建，24 小時內的問答會作為上下文附在新查詢前方
- **自動過期**：超過 24 小時無活動，下一次查詢自動以空白歷史重新開始（上限 20 輪）
- **新對話按鈕**：可隨時點擊「New conversation」立即清除歷史

## Agent Runtime

| Runtime | 設定值 | 預設 | 說明 |
|---------|--------|------|------|
| Claude Code | `claude` | yes | 支援 Claude hooks、`.claude/skills/` 與 `CLAUDE_ARGS` |
| OpenCode | `opencode` | no | 啟動 `opencode` CLI，使用 workspace 的 `.opencode/` |

在啟動時以 `AGENT_RUNTIME` 指定；CLI 參數（`CLAUDE_ARGS` / `OPENCODE_ARGS`）可在 Settings 熱改。

```bash
# Claude Code 跳過權限確認（在 Settings → Agent → Args 設定即可，不需重啟）
CLAUDE_ARGS=--dangerously-skip-permissions

# 使用 OpenCode
docker run -d -e AGENT_RUNTIME=opencode ...
```

## 排程器

在 terminal 中直接用自然語言告訴 Claude，例如：

> 「每天早上 9 點幫我做 daily standup 摘要」

Claude 會透過內建的 `local-schedule` skill 設定排程。排程資料存在 workspace 目錄，重啟容器後不遺失。

> 排程時間以容器時區為準，預設 UTC。若需台灣時間，啟動時加上 `-e TZ=Asia/Taipei`。

## Discord 整合

設定 `DISCORD_BOT_TOKEN` 後（可在 Settings 直接填，不需重啟），Perch 支援兩種模式：

### ACP 模式（推薦）

設定 `DISCORD_ACP_ENABLED=true`。每個 Discord channel 對應一個獨立的 subprocess，多輪對話上下文由 ACP session 保留。

```bash
npm install -g @agentclientprotocol/claude-agent-acp
```

### PTY 模式（預設）

每個 Discord channel 持有一個獨立的 Claude Code CLI PTY 行程。

### Hook 與 Reaction 對應

| Claude Hook 事件 | 行為 |
|------------------|------|
| 收到訊息 | 👀 |
| `PreToolUse` | ⚙️ |
| `PostToolUse` 成功 | ✅ |
| `PostToolUse` 失敗 | ❌ |
| `Stop`（回應完成）| 💬 + 文字訊息 |
| 回應超過 2000 字 | 📎 附件 |

### Discord Bot 設定

1. [Discord Developer Portal](https://discord.com/developers/applications) → New Application
2. Bot → Reset Token → 複製為 `DISCORD_BOT_TOKEN`
3. Privileged Gateway Intents → 開啟 **Message Content Intent**
4. OAuth2 → Scopes: `bot`；Bot Permissions: `View Channels`, `Send Messages`, `Read Message History`, `Add Reactions`
5. 複製邀請 URL，將 Bot 加入 Server

> **DM 安全提示：** DM 功能預設關閉。如需開啟，在 Settings 設定 `DISCORD_ALLOWED_USER_IDS`。

## Workspace Git Sync

自動定時將 `/workspace` 的 git repo 與 remote 同步（pull + push）。

```bash
docker run -d \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_SYNC_INTERVAL=60 \
  -e WORKSPACE_GIT_TOKEN=ghp_your_token \
  -e WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=your_discord_channel_id \
  ...
```

每次 sync 流程：偵測 rebase 狀態 → stash dirty 工作區 → `git pull --rebase` → submodule 更新（若啟用）→ stash pop → `git push`。

同一類型的錯誤在 5 分鐘內只通知一次（debounce）。

## License

MIT
