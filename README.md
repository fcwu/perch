# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。

---

## 亮點

### Web Terminal 排程

用自然語言告訴 Claude 設排程，直接生效。排程以 JSONL 存在 workspace，容器重啟不遺失。

![Web Terminal 排程設定](docs/images/schedule-setup.png)

### Web Terminal 監看 Discord

瀏覽器內切換 tab，即時觀看 Discord channel 的 Claude PTY 輸出。

![Web Terminal 監看 Discord](docs/images/discord-tab.png)

### Discord 排程觸發

排程在指定時間自動觸發，Discord channel 先出現來源提示，Claude 回覆以 thread 形式附在下方。

![Discord 排程回覆](docs/images/discord-reply.png)

---

## 功能

- **完整 terminal**：基於 xterm.js，支援顏色、滾動、可點擊的 URL
- **即時串流**：所有連線共用同一個 PTY session，即時看到 Claude Code 輸出
- **兩種運作模式**：單使用者（`PERCH_MODE=single`）直接存取 terminal；多使用者（`PERCH_MODE=multi`）透過 GitLab OAuth 管理多位使用者，管理員與一般使用者分流
- **四種認證方式**：無認證（內網測試）、密碼登入、mTLS 雙向憑證、GitLab OAuth
- **多輪對話 Chat UI**：網頁聊天介面支援連續追問，對話歷史自動從 SQLite 重建，24 小時內的對話可跨 session 保留
- **排程器**：用自然語言設定每天幾點自動送指令進 terminal（透過 `local-schedule` skill）
- **IP 封鎖**：TCP 層封鎖惡意 IP
- **限速**：HTTP 層限制登入/bootstrap 端點的請求頻率
- **自動重啟**：Claude Code 崩潰後自動重啟

---

## 推薦使用情境

### 情境一：個人知識庫（家用，推薦）

最低摩擦的個人使用方式：Perch 本身不做認證（`AUTH_METHOD=none`），存取控制交給 **Cloudflare Zero Trust** 處理，手機或外出時同樣可以安全連線。搭配 **Discord Bot** 讓你不用開瀏覽器也能隨時查詢。

**架構：**
```
手機 / 外出電腦
    │
    ▼
Cloudflare Zero Trust（身份驗證、mTLS 或 Email OTP）
    │
    ▼
Perch（AUTH_METHOD=none，只監聽 localhost 或內網）
    │
    ├─ Web Terminal（/）── 完整 Claude Code 操控
    └─ Discord Bot ──────── 隨時傳訊息查詢
```

**為什麼這樣搭配比密碼更好：**
- Cloudflare Zero Trust 提供 SSO / Google / GitHub 登入，不用自己管密碼
- Perch 完全不暴露在公網，即使 container 本身沒有認證也安全
- Discord Bot 讓手機查詢更自然，不需要開啟瀏覽器

```bash
# 家用個人模式：交給 Cloudflare Zero Trust 保護
docker run -d \
  -p 127.0.0.1:8080:8080 \
  -e PERCH_MODE=single \
  -e AUTH_METHOD=none \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -e DISCORD_BOT_TOKEN=<Bot Token> \
  -e DISCORD_CHANNEL_ID=<頻道 ID> \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

> `-p 127.0.0.1:8080:8080` 只綁定 localhost，Cloudflare Tunnel 連進來，外部無法直接存取。

**Cloudflare Zero Trust 設定步驟：**
1. [Cloudflare Zero Trust](https://one.dash.cloudflare.com/) → Networks → Tunnels → 建立 Tunnel，指向 `http://localhost:8080`
2. Access → Applications → 新增 Self-hosted App，設定允許的身份來源（Google、GitHub 等）
3. 完成後只有通過身份驗證的你才能透過 Cloudflare 連線到 Perch

**效果**：家裡 NAS 或樹莓派上執行 Perch，在任何地方用手機開瀏覽器直接進 terminal，或在 Discord 頻道傳訊息查詢。認證由 Cloudflare 管理，Perch 本身零設定。

---

### 情境二：多人知識庫（其他人只能查詢）

團隊共享 Perch：**操作員（管理員）**可控制 terminal；**其他成員**只能透過 Chat UI 查詢。

| 角色 | 設定方式 | 可存取路由 |
|------|---------|-----------|
| 管理員（你） | GitLab ID 加入 `GITLAB_ADMIN_IDS` | `/admin`（terminal + 管理面板） |
| 團隊成員 | GitLab 登入即可（`GITLAB_ALLOWED_IDS=*`） | `/chat`（只能聊天查詢） |
| 限定名單 | GitLab ID 加入 `GITLAB_ALLOWED_IDS` | `/chat`（只能聊天查詢） |

```bash
# 多人模式：管理員 + 開放所有 GitLab 使用者查詢
docker run -d \
  -p 8080:8080 \
  -e PERCH_MODE=multi \
  -e GITLAB_URL=https://gitlab.com \
  -e GITLAB_CLIENT_ID=<你的 App ID> \
  -e GITLAB_CLIENT_SECRET=<你的 App Secret> \
  -e GITLAB_REDIRECT_URI=https://your-domain/auth/callback \
  -e GITLAB_ADMIN_IDS=123456 \
  -e GITLAB_ALLOWED_IDS=* \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

若只想讓特定成員存取，將 `GITLAB_ALLOWED_IDS=*` 改為逗號分隔的 GitLab User ID：

```bash
-e GITLAB_ALLOWED_IDS=111111,222222,333333
```

**效果**：
- 管理員登入後進 `/admin`，可操作 terminal、查看歷史
- 其他成員登入後進 `/chat`，只能對 Claude 提問，無法操作 terminal
- 未登入者看到 GitLab 登入畫面，無法存取任何內容

---

## 快速開始

```bash
# 從 GitHub Container Registry 拉取
docker pull ghcr.io/fcwu/perch:latest

# 無認證模式（內網測試）
docker run -d \
  -p 8080:8080 \
  -e AUTH_METHOD=none \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest

# 密碼模式
docker run -d \
  -p 8080:8080 \
  -e AUTH_METHOD=password \
  -e PERCH_PASSWORD=你的密碼 \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest

# mTLS 模式（最安全，正式對外使用）
docker run -d \
  -p 8443:8443 \
  -e AUTH_METHOD=mtls \
  -e LISTEN_ADDR=:8443 \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

#### Mount 說明

| Mount | 用途 |
|-------|------|
| `-v ~/.claude:/home/perchuser/.claude` | Claude Code 設定、技能；含 `.credentials.json`（OAuth token） |
| `-v ~/.claude.json:/home/perchuser/.claude.json` | 記錄 `hasCompletedOnboarding` 與 `userID`；缺少時 Claude Code 視為全新安裝，即使憑證存在也會要求重新登入 |
| `-v ./:/workspace` | Claude Code 工作目錄（當前目錄）；排程資料也存於此 |

## 環境變數

### 核心設定

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `PERCH_MODE` | `single` | 運作模式：`single`（單使用者）/ `multi`（多使用者）；`multi` 需搭配 GitLab OAuth |
| `AUTH_METHOD` | `none` | 認證方式：`none` / `password` / `mtls` / `gitlab`；多使用者模式固定使用 `gitlab` |
| `PERCH_PASSWORD` | — | 密碼（`AUTH_METHOD=password` 時必填） |
| `LISTEN_ADDR` | `:8080` | 監聽位址；一般不需設定，mTLS 模式需改為 `:8443` |
| `PUID` | `1000` | 容器內行程的 UID；建議設為主機使用者的 `$(id -u)` |
| `PGID` | `PUID` 同值 | 容器內行程的 GID；建議設為主機使用者的 `$(id -g)` |
| `BLOCK_IPS` | — | 空格分隔的封鎖 IP 清單，支援 CIDR，例如 `1.2.3.4 10.0.0.0/8` |
| `AGENT_RUNTIME` | `claude` | Perch 啟動的 agent runtime：`claude` / `opencode` |
| `CLAUDE_WORKDIR` | `/workspace`（若存在） | Claude Code 的起始工作目錄 |
| `TZ` | `UTC` | 容器時區，影響排程觸發時間，例如 `Asia/Taipei` |

### Claude / OpenCode 設定

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `ANTHROPIC_API_KEY` | — | Anthropic API 金鑰，直接傳給 Claude |
| `CLAUDE_ARGS` | — | 傳給 `claude` 指令的額外 CLI 參數，例如 `--model claude-opus-4-5 --dangerously-skip-permissions` |
| `OPENCODE_ARGS` | — | 傳給 `opencode` 指令的額外 CLI 參數，例如 `-p "hello" -q` |
| `CLAUDE_CODE_NO_FLICKER` | `1` | 停用 Claude Code 的畫面閃爍動畫（設 `0` 可關閉） |
| `CLAUDE_CODE_DISABLE_MOUSE` | `1` | 停用 Claude Code 的滑鼠事件捕捉（設 `0` 可關閉） |

### GitLab OAuth

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `GITLAB_URL` | — | GitLab instance URL，例如 `https://gitlab.example.com` |
| `GITLAB_CLIENT_ID` | — | GitLab OAuth Application 的 Client ID |
| `GITLAB_CLIENT_SECRET` | — | GitLab OAuth Application 的 Client Secret |
| `GITLAB_REDIRECT_URI` | — | OAuth callback URI，例如 `https://perch.example.com/auth/callback` |
| `GITLAB_ADMIN_IDS` | — | 逗號分隔的 GitLab 使用者 ID，這些 ID 具有管理員權限；多使用者模式中會被路由到 `/admin`，單使用者 GitLab 模式中作為允許名單 |
| `GITLAB_ALLOWED_IDS` | — | 逗號分隔的 GitLab 使用者 ID（多使用者模式）：空白=拒絕所有一般使用者，`*`=允許所有已認證使用者，逗號清單=只允許指定 ID |
| `COOKIE_SECRET` | （固定預設值） | 用於簽署 `perch_session` cookie 的 HMAC 密鑰；**正式環境請務必設定隨機值** |

### Admin 與儲存

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `ADMIN_TOKEN` | — | Admin 管理介面的 token；設定後 `/admin` 路由啟用（即時監控、歷史查詢、使用量統計） |
| `DB_PATH` | `/data/perch.db` | SQLite 資料庫路徑，用於持久化查詢紀錄；GitLab OAuth 啟用時自動使用預設路徑 |
| `RATE_LIMIT_RPM` | `10` | 每位使用者每分鐘最多查詢次數；設為 `0` 停用限速 |
| `LOG_FORMAT` | `text` | Log 輸出格式：`text`（人讀格式）或 `json`（結構化，方便 ELK/Loki 收集） |

### Discord 整合

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `DISCORD_BOT_TOKEN` | — | Discord bot token（啟用 Discord 整合） |
| `DISCORD_CHANNEL_ID` | — | **選填**。限制只監聽指定 channel ID；不設定時監聽所有頻道 |
| `DISCORD_ALLOWED_USER_IDS` | — | **選填**。逗號分隔的 Discord 用戶 ID 白名單；**未設定時 DM 功能完全關閉** |
| `DISCORD_ACP_ENABLED` | — | 設為 `true` 啟用 ACP stdio 模式（推薦）；未設定時使用 PTY 模式 |
| `ACP_EXECUTABLE` | `claude-agent-acp` | ACP subprocess 執行檔路徑 |
| `ACP_RUN_TIMEOUT` | `300` | 每個 prompt 的逾時秒數 |

### Workspace Git Sync

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `WORKSPACE_GIT_SYNC_ENABLED` | `false` | 設為 `true` 或 `1` 啟用 workspace 自動 git sync |
| `WORKSPACE_GIT_SYNC_INTERVAL` | `60` | Sync 間隔秒數 |
| `WORKSPACE_PATH` | `/workspace` | 要同步的 git repo 路徑 |
| `WORKSPACE_GIT_TOKEN` | — | HTTPS remote 的 git token |
| `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` | — | 同步失敗時送通知的 Discord channel ID |
| `WORKSPACE_GIT_SYNC_SUBMODULES` | `false` | 設為 `true` 或 `1` 在每次 pull 後自動執行 submodule update |

---

## 運作模式

### 單使用者模式（預設）

`PERCH_MODE=single`（或不設定）啟動單使用者模式。`/` 直接提供 terminal UI，認證方式由 `AUTH_METHOD` 決定。

### 多使用者模式

`PERCH_MODE=multi` 啟動多使用者模式，需搭配 GitLab OAuth。

- **管理員**（`GITLAB_ADMIN_IDS` 中的使用者）：登入後路由到 `/admin`（terminal UI + 管理面板），也可直接瀏覽 `/chat`
- **一般使用者**（`GITLAB_ALLOWED_IDS` 控制）：登入後路由到 `/chat`（多輪對話 Chat UI）
- **未認證的訪客**：`/`、`/chat`、`/admin` 均顯示 GitLab 登入畫面，不進行伺服器端重導向

**啟動範例：**

```bash
docker run -d \
  -p 8080:8080 \
  -e PERCH_MODE=multi \
  -e AGENT_RUNTIME=opencode \
  -e GITLAB_URL=https://gitlab.example.com \
  -e GITLAB_CLIENT_ID=your-client-id \
  -e GITLAB_CLIENT_SECRET=your-client-secret \
  -e GITLAB_REDIRECT_URI=https://perch.example.com/auth/callback \
  -e GITLAB_ADMIN_IDS=101,202 \
  -e GITLAB_ALLOWED_IDS=* \
  -e COOKIE_SECRET=$(openssl rand -hex 32) \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

---

## Chat UI（多輪對話知識庫查詢）

Chat UI 提供多輪對話支援，使用者可以追問後續問題，agent 會自動記住過去 24 小時內的對話歷史。

- **多輪對話**：對話歷史從 SQLite 自動重建，24 小時內的問答會作為上下文附在新查詢前方
- **自動過期**：超過 24 小時無活動，下一次查詢自動以空白歷史重新開始（上限 20 輪）
- **新對話按鈕**：可隨時點擊「New conversation」立即清除歷史，伺服器端同步跳過歷史查詢
- **對話串渲染**：所有輪次以可捲動的 user/assistant 氣泡呈現
- **登出**：所有已認證頁面均顯示登出按鈕，點擊後清除 session 並返回登入畫面

### 多使用者模式啟用 Chat UI

同上方「多使用者模式」啟動範例，一般使用者登入後自動路由至 `/chat`。

### 單使用者模式啟用 Chat UI（搭配 GitLab 認證）

```bash
docker run -d \
  -p 8080:8080 \
  -e PERCH_MODE=single \
  -e AUTH_METHOD=gitlab \
  -e AGENT_RUNTIME=opencode \
  -e GITLAB_URL=https://gitlab.example.com \
  -e GITLAB_CLIENT_ID=your-client-id \
  -e GITLAB_CLIENT_SECRET=your-client-secret \
  -e GITLAB_REDIRECT_URI=https://perch.example.com/auth/callback \
  -e COOKIE_SECRET=$(openssl rand -hex 32) \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

---

## 認證方式說明

### `AUTH_METHOD=none` — 無認證

無任何驗證，所有人可直接連線。**僅限內網或本地測試使用**，絕對不要暴露在公網。

### `AUTH_METHOD=password` — 密碼登入

連線後需輸入密碼才能看到 terminal。密碼以 cookie session 方式儲存。需設定 `PERCH_PASSWORD`。

### `AUTH_METHOD=mtls` — 雙向 TLS（mTLS）

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

### `AUTH_METHOD=gitlab` — GitLab OAuth（單使用者）

使用 GitLab OAuth 作為單使用者認證。需設定 `GITLAB_URL`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`。

若設定 `GITLAB_ADMIN_IDS`，只有清單中的 GitLab 使用者 ID 可以通過認證（作為允許名單）；若不設定，所有已認證的 GitLab 使用者均可登入。

---

## 公開 API Endpoint

| Endpoint | 說明 |
|----------|------|
| `GET /api/auth/status` | 回傳當前認證狀態（始終 HTTP 200）：`{"authenticated": bool, "username": "", "role": "admin\|user\|", "mode": "single\|multi"}` |
| `GET /auth/logout` | 清除 session cookie，重導向至 `/` |
| `GET /auth/gitlab` | 開始 GitLab OAuth 流程 |
| `GET /auth/callback` | GitLab OAuth callback |

---

## Agent Runtime

Perch 現在支援兩種 agent runtime：

| Runtime | 設定值 | 預設 | 說明 |
|---------|--------|------|------|
| Claude Code | `claude` | yes | 保留既有行為，支援 Claude hooks、`.claude/skills/` 與 `CLAUDE_ARGS` |
| OpenCode | `opencode` | no | 啟動 `opencode` CLI，使用 workspace 的 `.opencode/` 設定資產與 `OPENCODE_ARGS` |

### 使用 OpenCode

```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_METHOD=none \
  -e AGENT_RUNTIME=opencode \
  -e OPENCODE_ARGS="-q" \
  -e ANTHROPIC_API_KEY=<your-key> \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

OpenCode 會使用 project-level `.opencode/` 目錄。Perch 會在 container 啟動時把 image 內建的 OpenCode assets 複製到 workspace 的 `.opencode/`，不會去修改 `~/.claude/settings.json`。

## Claude 啟動設定

### 傳入 CLI 參數

透過 `CLAUDE_ARGS` 可以在啟動時將額外參數傳給 `claude` 指令：

```bash
# 指定模型
docker run -d \
  -e CLAUDE_ARGS="--model claude-opus-4-5" \
  ...

# 跳過權限確認（適合全自動場景）
docker run -d \
  -e CLAUDE_ARGS="--dangerously-skip-permissions" \
  ...

# 同時多個參數
docker run -d \
  -e CLAUDE_ARGS="--model claude-opus-4-5 --dangerously-skip-permissions" \
  ...
```

### 覆蓋預設環境變數

Perch 預設替 Claude 設定兩個環境變數，可透過 Docker `-e` 直接覆蓋：

| 變數 | 預設 | 說明 |
|------|------|------|
| `CLAUDE_CODE_NO_FLICKER` | `1` | 停用畫面閃爍動畫，減少 terminal 雜訊 |
| `CLAUDE_CODE_DISABLE_MOUSE` | `1` | 停用滑鼠事件，讓瀏覽器文字選取正常運作 |

```bash
# 例：恢復滑鼠事件（若你的用途需要）
docker run -d \
  -e CLAUDE_CODE_DISABLE_MOUSE=0 \
  ...
```

> Perch 傳給 Claude 的所有環境變數均繼承自容器環境，任何以 `-e` 設定的變數都會直接傳入 Claude 行程。

---

## 排程器

Perch 內建排程功能，可以設定每天特定時間自動送指令進 terminal（例如：每天早上 9 點叫 Claude 做 daily review）。

在 terminal 中直接用自然語言告訴 Claude，例如：

> 「每天早上 9 點幫我做 daily standup 摘要」

Claude 會透過內建的 `local-schedule` skill 設定排程。排程資料存在 workspace 目錄，重啟容器後不遺失。

> 排程時間以容器時區為準，預設 UTC。若需台灣時間，啟動時加上 `-e TZ=Asia/Taipei`。

---

## Discord 整合

Perch 支援兩種 Discord 處理模式：**PTY 模式**（預設）和 **ACP 模式**（`DISCORD_ACP_ENABLED=true`）。

### ACP 模式（推薦）

設定 `DISCORD_ACP_ENABLED=true` 啟用 ACP stdio 模式。Perch 直接管理 `claude-agent-acp` subprocess，每個 Discord channel 對應一個獨立的 subprocess，多輪對話上下文由 ACP session 保留。Subprocess crash 後下一則訊息自動重啟，不需重啟 Perch。

**安裝 claude-agent-acp：**

```bash
npm install -g @agentclientprotocol/claude-agent-acp
```

**PTY 模式**（`DISCORD_ACP_ENABLED` 未設）：每個 Discord channel 持有一個獨立的 Claude Code CLI PTY 行程，訊息從 Discord 進來寫進 PTY，再由 agent runtime 處理後回傳。

---

訊息從 Discord 進來，寫進 PTY（PTY 模式）或 ACP subprocess（ACP 模式），再由 agent runtime 處理後回傳到 Discord。

### Hook 與 Reaction 對應

`AGENT_RUNTIME=claude` 時，Perch 使用 Claude Code hooks 來驅動 reaction 與 completion 回覆。

`AGENT_RUNTIME=opencode` 時，OpenCode 沒有直接沿用同一套 hook 協定，Perch 會改用 PTY output idle fallback 偵測完成並送出最後回覆。這代表 OpenCode 模式下沒有 `PreToolUse` / `PostToolUse` reaction 細節，但仍保留 Discord request/reply 流程。

| Claude Hook 事件 | 行為 |
|------------------|------|
| 收到訊息（進入 PTY）| 👀 |
| `PreToolUse` | ⚙️ |
| `PostToolUse` 成功 | ✅ |
| `PostToolUse` 失敗 | ❌ |
| `Stop`（回應完成）| 💬 + 文字訊息 |
| 回應超過 2000 字 | 📎 附件 |

---

## Discord Bot 設定

### 步驟一：建立 Bot

1. 前往 [Discord Developer Portal](https://discord.com/developers/applications)
2. 點 **New Application** → 輸入名稱（例如 `perch`）→ Create
3. 左側選 **Installation** → 取消勾選 **User Install**，保留 **Guild Install**，**Install Link** 選 **None**
4. 左側選 **Bot** → 點 **Add Bot**
5. 在 **TOKEN** 區塊點 **Reset Token** → 複製 token → 存為 `DISCORD_BOT_TOKEN`
6. 在同一頁找 **Authorization Flow** → 關閉 **Public Bot**
7. 在同一頁往下找 **Privileged Gateway Intents** → 開啟 **Message Content Intent**

### 步驟二：邀請 Bot 進 Server

1. 左側選 **OAuth2**
2. Scopes 勾選：`bot`
3. Bot Permissions 勾選：
   - `View Channels`
   - `Send Messages`
   - `Create Public Threads`（目前未使用）
   - `Send Messages in Threads`（目前未使用）
   - `Manage Messages`（目前未使用）
   - `Manage Threads`（目前未使用）
   - `Read Message History`
   - `Add Reactions`
4. 複製頁面下方產生的 URL → 在瀏覽器開啟 → 選擇要加入的 Server → Authorize

### 步驟三：取得 Channel ID（選填）

不設定 `DISCORD_CHANNEL_ID` 時，Bot 會監聽所有有權限的頻道，行為如下：

| 頻道類型 | 觸發方式 |
|----------|----------|
| 公開頻道（@everyone 可見） | 需要 @mention Bot |
| 私密頻道（@everyone 不可見） | 直接對話，不需 @mention |
| DM（私訊） | 需設定 `DISCORD_ALLOWED_USER_IDS`，未設定時 **DM 功能關閉** |

> **安全提示：** DM 功能預設關閉。任何知道 Bot 用戶 ID 的人都可以傳送 DM，若未限制則任何人皆可控制 Claude Code。如需開啟 DM，請透過 `DISCORD_ALLOWED_USER_IDS` 明確指定允許的用戶 ID：
> ```
> -e DISCORD_ALLOWED_USER_IDS=你的Discord用戶ID
> ```

如果只想監聽單一頻道：

1. Discord 開啟 **User Settings → Advanced** → 啟用 **Developer Mode**
2. 右鍵點擊要監聽的 channel → **Copy Channel ID** → 存為 `DISCORD_CHANNEL_ID`

### 步驟四：啟動

**Open-channel 模式**（推薦）：

```bash
docker run -d \
  ...
  -e DISCORD_BOT_TOKEN=your_bot_token \
  ...
  ghcr.io/fcwu/perch:latest
```

**指定單一頻道模式**（向下相容）：

```bash
docker run -d \
  ...
  -e DISCORD_BOT_TOKEN=your_bot_token \
  -e DISCORD_CHANNEL_ID=your_channel_id \
  ...
  ghcr.io/fcwu/perch:latest
```

---

## Workspace Git Sync

Perch 可以自動定時將 `/workspace` 的 git repo 與 remote 同步（pull + push），讓 Claude 的工作成果即時備份到 remote，也能在多個 container 間共享最新狀態。

### 啟用方式

```bash
docker run -d \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_SYNC_INTERVAL=60 \
  -e WORKSPACE_GIT_TOKEN=ghp_your_token \
  -e WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=your_discord_channel_id \
  -v ./:/workspace \
  ghcr.io/fcwu/perch:latest
```

### 行為說明

每隔 `WORKSPACE_GIT_SYNC_INTERVAL` 秒執行一次 sync：

1. **偵測 rebase 狀態**：若 `.git/rebase-merge` 或 `.git/rebase-apply` 存在，執行 `git rebase --abort` 並通知 Discord
2. **Stash dirty 工作區**：若有未提交的變更，先 `git stash`
3. **Pull rebase**：執行 `git pull --rebase`
4. **Submodule 更新**（若啟用）：執行 `git submodule update --init --recursive`
5. **Stash pop**：若 step 2 有 stash，還原工作區
6. **Push**：執行 `git push`

### HTTPS Token 設定

`WORKSPACE_GIT_TOKEN` 只適用於 HTTPS remote。啟動時 Perch 會自動：

- 將 token 寫入 `~/.git-credentials`（格式：`https://x-token-auth:<token>@<host>`）
- 執行 `git config --global credential.helper store`

SSH remote 不需要 token，設定了也會被忽略（並記錄 Warning）。

### 錯誤處理與通知

| 情況 | 行為 |
|------|------|
| rebase 衝突 | abort rebase → Discord 通知（建議手動 pull） |
| pull 失敗 | 記錄 log → Discord 通知（含 git 輸出） |
| stash pop 失敗 | 記錄 log + stash ref → Discord 通知 |
| push 失敗 | 記錄 log → Discord 通知（含 git 輸出） |
| submodule 更新失敗 | 記錄 log → Discord 通知，不中斷後續 sync |

同一類型的錯誤在 **5 分鐘內只通知一次**（debounce），避免每 60 秒重複轟炸 Discord。

### Submodule 支援

```bash
-e WORKSPACE_GIT_SYNC_SUBMODULES=true
```

啟用後，每次 `git pull --rebase` 成功後會自動執行：

```
git submodule update --init --recursive
```

submodule 更新失敗只記錄 log + Discord 通知，不影響主 repo 的 push。

---

## License

MIT
