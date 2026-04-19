# Perch Debug 指南

通用除錯與部署方法。環境特定值（主機 IP、容器名稱、路徑）記錄在 `docs/.env.<device>.md`（本機專用，不進 git）。

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

## 3. 本機 debug 連線（SSH port forward）

若需要從本機瀏覽器測試遠端服務（特別是 OAuth callback URL 必須與遠端 FQDN 一致時）：

**Step 1：SSH port forward**
```bash
ssh -L <local-port>:localhost:<remote-port> <user>@<host-ip>
```
> 用直連 IP，不用 FQDN（防火牆可能擋 22 port）

**Step 2：修改 /etc/hosts（若需要 FQDN）**
```
127.0.0.1  <fqdn>
```

**Step 3：瀏覽器用 `<fqdn>:<local-port>` 開啟**

原因：`/etc/hosts` 讓瀏覽器把 FQDN 視為 localhost，Chrome PNA 政策不觸發，且 OAuth redirect URI 與 GitLab 設定吻合。

---

## 4. 瀏覽器自動化（chrome-cdp）

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
node $CDP click <target> <selector>    # 點擊元素
node $CDP type  <target> <text>        # 輸入文字
```

> target ID 每次重啟 Chrome 會變，用 `list` 重新查。

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

---

## 6. 常見環境限制

| 限制 | 說明 | 對策 |
|------|------|------|
| Squid 透明 proxy | WebSocket `Upgrade` header 被剝除 | Chat 改用 SSE |
| Chrome PNA | 公開 FQDN 解析到私有 IP，WS 101 被擋 | 帶 `Access-Control-Allow-Private-Network: true` header |
| Container /tmp 空間 | tmpfs 通常只有 128MB | 大檔案用 host 上有空間的目錄中轉 |
| OAuth origin | port forward 時 callback domain 可能不符 | 用 /etc/hosts 讓 FQDN 指向 127.0.0.1 |
