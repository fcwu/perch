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

# 確認 entrypoint seed 是否成功（grep 關鍵字）
docker logs <container> 2>&1 | grep "perch entrypoint:"

# 確認 /tmp 空間（容器內 tmpfs 通常只有 128MB）
docker exec <container> df -h /tmp
# 若空間不足，用 host 上其他目錄中轉大檔案
```

---

## 3. 瀏覽器自動化（chrome-cdp）

使用 `chrome-cdp` skill 搭配專用 Chrome agent instance，**不使用用戶主 Chrome**（避免跳出授權對話框）。

### 啟動專用 Chrome（測試前必做）

```bash
# 啟動（已在跑就直接略過）
tests/chrome-agent.sh

# 停止
tests/chrome-agent.sh stop
```

`CDP_PORT_FILE` 已在 `settings.local.json` 設定為 `tests/.chrome-agent/DevToolsActivePort`，`chrome-cdp` skill 會自動使用此 instance，無需額外設定。

### CDP 操作

```bash
CDP=~/.claude/skills/chrome-cdp/scripts/cdp.mjs

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

## 4. 瀏覽器網路面板快速診斷

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

## 5. 附件落盤路徑（agent 端）

非圖檔附件由 server 寫到 `<workdir>/uploads/<conv-id>/<filename>`，並在 prompt 最前面加上：

```
[file: ./uploads/<conv-id>/<filename> (<mime>, <size>)]

<原使用者文字>
```

Agent 看到此前綴時，路徑相對 `<workdir>`，可直接用 `Read`、`Bash`（含 `pdftotext`、`jq`、`wc` 等）讀取分析。Discord 的「conv-id」是 channel ID。

清理：

- ACP session pool 把 (user, conv) 從 pool evict 時，`uploads/<conv-id>/` 會被刪掉
- perch 啟動會掃所有子目錄的 mtime，超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS`（預設 7）天的整個刪掉
- 容器重啟不會清掉「最近還在用」的目錄

