# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。


<!-- @import "[TOC]" {cmd="toc" depthFrom=2 depthTo=6 orderedList=false} -->

<!-- code_chunk_output -->

- [功能](#功能)
  - [Web Terminal 排程](#web-terminal-排程)
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
  - [Reaction 對應](#reaction-對應)
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

### Discord 排程觸發

排程在指定時間自動觸發，Discord channel 先出現來源提示，Claude 回覆以 thread 形式附在下方。

![Discord 排程回覆](docs/images/discord-reply.png)

## 快速開始

```bash
docker run -d \
  -p 127.0.0.1:8080:8080 \
  -e PUID=$(id -u) -e PGID=$(id -g) -e TZ=Asia/Taipei \
  -v ~/.claude:/etc/perch-claude-host:ro \
  -v ./:/workspace \
  -v ./data:/data \
  ghcr.io/fcwu/perch:latest
```

瀏覽器開啟 `http://localhost:8080`，點右上角齒輪圖示進入 **Settings** 完成後續設定。

### Mount 說明

| Mount | 用途 |
|-------|------|
| `-v ~/.claude:/etc/perch-claude-host:ro` | （建議）把 host 上的 Claude Code 設定（credentials、plugins、skills）帶進來；不掛則需在 web terminal 跑一次 `claude /login` |
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
| `AGENT_RUNTIME` | `claude` | `claude` / `opencode` / `codex` |
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
| `WORKSPACE_GIT_TOKEN` | — | HTTPS remote 的 git token（SSH remote 請略過） |
| `WORKSPACE_GIT_USER_NAME` | — | git commit 的 user.name |
| `WORKSPACE_GIT_USER_EMAIL` | — | git commit 的 user.email |
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
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | Telegram chat ID |
| `CHAT_UPLOAD_MAX_BYTES` | `10485760` (10 MiB) | 單一附件大小上限 |
| `CHAT_UPLOAD_MAX_FILES` | `4` | 單次 query 附件數量上限 |
| `CHAT_UPLOAD_ALLOWED_MIME` | image+text+pdf 預設集 | 允許的 MIME 白名單（逗號分隔） |
| `CHAT_UPLOAD_DIR_QUOTA_BYTES` | `524288000` (500 MiB) | 每個 conversation 累計上傳容量上限 |
| `CHAT_UPLOAD_ORPHAN_TTL_DAYS` | `7` | 啟動時 mtime 超過 N 天的孤兒 uploads 目錄自動刪除 |

### 附件處理

- **圖片**（PNG/JPEG/GIF/WebP）：直接送入 AI 視覺理解
- **文件**（TXT、Markdown、CSV、JSON、PDF 等）：agent 讀取後分析，適合丟入程式碼、日誌、報表等檔案
- 上傳的檔案在對話結束後自動清理

---

## 推薦使用情境

### 情境一：個人知識庫（家用，推薦）

Perch 本身不做認證（`AUTH_METHOD=none`），存取控制交給 **Cloudflare Zero Trust** 處理。搭配 **Discord Bot** 讓你不用開瀏覽器也能隨時查詢。

```bash
docker run -d \
  -p 127.0.0.1:8080:8080 \
  -e PUID=$(id -u) -e PGID=$(id -g) -e TZ=Asia/Taipei \
  -v ~/.claude:/etc/perch-claude-host:ro \
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
  -v ~/.claude:/etc/perch-claude-host:ro \
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
- **上傳附件**：textarea 旁的 📎 開啟 file picker；也支援把檔案**拖進**輸入框，圖片還可以 Cmd/Ctrl+V **貼**剪貼簿截圖。支援圖片（PNG/JPEG/GIF/WebP）與文件（TXT/Markdown/CSV/JSON/PDF 等），預設最多 4 個、單檔 10MB。Discord 直接 attach 檔案給 bot 也支援。

## Agent Runtime

| Runtime | 設定值 | 預設 | 說明 |
|---------|--------|------|------|
| Claude Code | `claude` | yes | 支援 `.claude/skills/` 與 `CLAUDE_ARGS` |
| OpenCode | `opencode` | no | 支援 `.opencode/skills/` 與 `OPENCODE_ARGS` |
| Codex | `codex` | no | 支援 `.codex/skills/` 與 `CODEX_ARGS`；auth 走 `OPENAI_API_KEY` 或 ChatGPT OAuth |

啟動時以 `AGENT_RUNTIME` 指定；切換需重啟（`AGENT_RUNTIME` 在啟動才讀，runtime-time 不變）。CLI 參數（`CLAUDE_ARGS` / `OPENCODE_ARGS` / `CODEX_ARGS`）可在 Settings 熱改。

> **runtime 影響範圍**
> - **Web terminal（`/ws`）**：直接 spawn 對應的互動式 CLI（PTY）
> - **Chat API（`/chat`）、Discord、Telegram**：透過 ACP subprocess 處理對話
>
> 切換 runtime 兩者都會跟著變。

```bash
# Claude Code 跳過權限確認（在 Settings → Agent → Args 設定即可，不需重啟）
CLAUDE_ARGS=--dangerously-skip-permissions

# 使用 OpenCode
docker run -d -e AGENT_RUNTIME=opencode ...

# 使用 Codex（OpenAI）
docker run -d -e AGENT_RUNTIME=codex -e OPENAI_API_KEY=sk-... ...
```

### OpenCode 額外注意

- **免費模型 credential-less**：`opencode/gpt-5-nano`、`opencode/hy3-preview-free` 等可直接用，不需登入
- **付費模型（Anthropic、OpenAI、Google 等）需登入**：在 host 跑一次 `opencode auth login`，將 `~/.local/share/opencode` 掛進容器；或直接在容器內以 `docker exec -it ... opencode auth login` 完成 OAuth flow（auth.json 寫進掛載 volume 才會持久）
- **mode 差異**：OpenCode 用 `build`/`plan` mode（不是 Claude 的 `bypassPermissions`/`acceptEdits`）。perch 仍會嘗試 set `bypassPermissions`，opencode 會 reject 並 warning，但 default `build` mode 已能執行 tools，**不影響功能**

### Codex 額外注意

- **API Key 認證**：設 `OPENAI_API_KEY`（或 `CODEX_API_KEY`）即可，無需登入
- **ChatGPT OAuth 認證**：在 host 跑一次 `codex login`，將 `~/.codex` 掛進容器；或直接在容器內以 `docker exec -it ... codex login` 完成 OAuth flow（認證檔案需寫進掛載 volume 才會持久）
- **預設 read-only**：Codex 預設只能讀取檔案；要修改檔案或執行指令，Codex 會逐步詢問每個操作的授權
- **認證錯誤會顯示在 chat**：若 API Key 無效或認證未完成，錯誤訊息會顯示在 chat 視窗

### Advanced overrides（dev / debugging）

| 變數 | 用途 |
|------|------|
| `ACP_EXECUTABLE` | 覆蓋 `runtime.ACPExecutable`（指 fork 路徑或 mock subprocess）|
| `ACP_EXECUTABLE_ARGS` | 覆蓋 `runtime.ACPArgs`（JSON array，如 `["acp","--log-level","DEBUG"]`）|

## 排程器

在 terminal 中直接用自然語言告訴 Claude，例如：

> 「每天早上 9 點幫我做 daily standup 摘要」

Claude 會透過內建的 `local-schedule` skill 設定排程。排程資料存在 workspace 目錄，重啟容器後不遺失。

> 排程時間以容器時區為準，預設 UTC。若需台灣時間，啟動時加上 `-e TZ=Asia/Taipei`。

## Discord 整合

設定 `DISCORD_BOT_TOKEN` 後（可在 Settings 直接填，不需重啟），即可在 Discord 與 Claude 對話。每個 channel 是獨立的對話，會記住前後文。

### Reaction 對應

| 狀態 | Reaction |
|------|----------|
| 收到訊息 | 👀 |
| 回應完成 | 💬 + 文字訊息 |
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

**前提：** `/workspace`（或 `WORKSPACE_PATH`）必須已是設好 remote 的 git repo。Perch 啟動時若偵測不到 `.git/` 目錄，sync 功能會跳過。

每次 sync 流程：
1. 偵測 rebase 狀態 → 若在 rebase 中先執行 `git rebase --abort`
2. 若工作區有未提交的變更 → 自動 `git add -A` + `git commit -m "auto-sync: <timestamp>"`
3. `git pull --rebase`
4. submodule 更新（若 `WORKSPACE_GIT_SYNC_SUBMODULES=true`）
5. `git push`

同一類型的錯誤在 5 分鐘內只通知一次（debounce）。

### HTTPS Remote（GitHub / GitLab）

```bash
docker run -d \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_TOKEN=ghp_your_token \
  -e WORKSPACE_GIT_USER_NAME="Your Name" \
  -e WORKSPACE_GIT_USER_EMAIL="you@example.com" \
  -e WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=your_discord_channel_id \
  ...
```

`WORKSPACE_GIT_TOKEN` 會寫入容器內的 `~/.git-credentials`，remote 須為 HTTPS。

### SSH Remote

```bash
docker run -d \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_USER_NAME="Your Name" \
  -e WORKSPACE_GIT_USER_EMAIL="you@example.com" \
  -v ~/.ssh:/home/perchuser/.ssh:ro \
  ...
```

remote 為 `git@...` 或 `ssh://` 時，`WORKSPACE_GIT_TOKEN` 自動略過，直接使用 SSH key。

### Local Bare Repo（NAS / 本機 file://）

若 bare repo 在本機或 NAS 上，可直接掛載進容器，不需 token：

```yaml
# docker-compose.yml
volumes:
  - /path/to/your/repo.git:/mykb.git   # 掛載 bare repo
  - /path/to/workspace:/workspace
```

```env
# .env
WORKSPACE_GIT_SYNC_ENABLED=true
WORKSPACE_GIT_USER_NAME=Your Name
WORKSPACE_GIT_USER_EMAIL=you@example.com
```

workspace 內的 git remote 設為容器內路徑：

```bash
# 首次設定（在 NAS 或容器內執行）
git -C /workspace remote set-url origin file:///mykb.git
```

## License

MIT
