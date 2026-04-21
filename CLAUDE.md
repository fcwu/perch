# Perch Debug 指南

通用除錯與部署方法。環境特定值（主機 IP、容器名稱、路徑）記錄在 `tests/.env.<device>.md`（本機專用，不進 git）。

---

## 1. 編譯與部署

### 編譯

```bash
# 前端
cd frontend && npm run build && cd ..

# Go binary（本機）
go build -o perch .

# Go binary（Linux container 用）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o perch_linux_amd64 .
```

### 部署到遠端容器（不 rebuild image）

```bash
# 傳 binary 到遠端
scp perch_linux_amd64 <user>@<host>:<staging-path>/

# 複製進容器並重啟（保留 volume）
ssh <user>@<host> "docker cp <staging-path>/perch_linux_amd64 <container>:/app/perch && docker restart <container>"

# 確認
ssh <user>@<host> "docker logs <container> --tail 10"
```

### Recreate 容器（改了 .env）

```bash
cd <deploy-dir>
docker compose -f docker-compose.local.yml down && docker compose -f docker-compose.local.yml up -d
```

---

## 2. 容器除錯

```bash
# 進入容器
docker exec -it <container> sh

# 確認環境變數
docker inspect <container> | jq '.[0].Config.Env'

# 查看 log
docker logs <container> --tail 50 -f

# 確認 /tmp 空間（容器內 tmpfs 通常只有 128MB）
docker exec <container> df -h /tmp
# 若空間不足，用 host 上其他目錄中轉大檔案
```

---

## 3. 瀏覽器自動化（chrome-cdp）

使用 `chrome-cdp` skill（需先在 Chrome 開啟 remote debugging）。

```bash
CDP=/Users/dorowu/.claude/skills/chrome-cdp/scripts/cdp.mjs

# 列出分頁
node $CDP list

# 常用操作
node $CDP nav   <target> <url>         # 導航
node $CDP shot  <target> [file]        # 截圖
node $CDP snap  <target>               # accessibility tree
node $CDP eval  <target> <expr>        # 執行 JS
node $CDP click   <target> <selector>              # 點擊元素
node $CDP type    <target> <text>                  # 輸入文字
node $CDP evalraw <target> Network.getCookies '{"urls":["<url>"]}' # 查 cookies
```

> target ID 每次重啟 Chrome 會變，用 `list` 重新查。

---

## 4. Discord ACP 模式

設定 `DISCORD_ACP_ENABLED=true` 以啟用 ACP stdio 模式；未設定時 Discord 維持 PTY 模式。

Perch 直接管理 `claude-agent-acp` subprocess，每個 Discord channel 對應一個獨立的 subprocess，多輪對話上下文由 ACP session 保留。不需要外部 bridge service。

### 安裝 claude-agent-acp

Dockerfile 的 Node.js stage 中加入：

```dockerfile
RUN npm install -g @agentclientprotocol/claude-agent-acp
```

本機測試：

```bash
npm install -g @agentclientprotocol/claude-agent-acp
```

### 環境變數

| 環境變數 | 說明 | 預設值 |
|---------|------|--------|
| `DISCORD_ACP_ENABLED` | 設為 `true` 啟用 ACP stdio 模式 | 未設 = PTY 模式 |
| `ACP_EXECUTABLE` | ACP subprocess 執行檔路徑 | `claude-agent-acp` |
| `ACP_RUN_TIMEOUT` | 每個 prompt 的逾時秒數 | `300`（5 分鐘）|

### 模式說明

**PTY 模式**（`DISCORD_ACP_ENABLED` 未設）：每個 Discord channel 持有一個獨立的 Claude Code CLI PTY 行程。

**ACP 模式**（`DISCORD_ACP_ENABLED=true`）：Perch fork `claude-agent-acp` subprocess，透過 ACP JSON-RPC over stdio 通訊。每個 channel 一個 subprocess，subprocess crash 後下一則訊息自動重啟。`permissionMode: "bypassPermissions"` 已在 `new_session` 時傳入，等效於 `--dangerously-skip-permissions`。

### 架構

```
Discord message
    → discordSession.handleWithACP()
        → ACPProcess.EnsureRunning()   (lazy start / crash recovery)
        → ACPProcess.Prompt(text)
            ┌─ write: {jsonrpc, id, method:"prompt", params:{sessionId, content}}
            │         → claude-agent-acp stdin
            │
            │  claude-agent-acp stdout →
            ├─ read:  agent_message_chunk notifications  (accumulated)
            └─ read:  response {result:{status:"completed"}}
        → return accumulated text
    → split & send Discord reply
```

### 快速測試

```bash
# 驗證 claude-agent-acp binary 可用
claude-agent-acp --version

# 手動模擬 ACP handshake（確認 JSON-RPC 格式正確）
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-05-16","clientInfo":{"name":"perch","version":"1.0"}}}' \
  | claude-agent-acp
```

---

## 5. 瀏覽器網路面板快速診斷

| 現象 | 意義 |
|------|------|
| WS status 200（非 101）| Proxy 剝除 `Upgrade` header |
| WS status 403 | Auth middleware 擋住 |
| SSE 連線立刻關閉 | 後端沒有對應 session（先打 POST /api/chat）|
| 409 Conflict | 已有 session 在執行中 |
| Set-Cookie domain 不對 | OAuth state cookie 設在錯誤 origin |

```bash
# 快速用 curl 測試 SSE
curl -v -N -H "Cookie: session_token=<token>" http://localhost:<port>/api/chat/stream
# 若看到 "data:" 開頭的行 → SSE 正常
```

