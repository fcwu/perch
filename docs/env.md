# Perch Debug 技巧筆記

本文記錄在偵錯 Perch（WebSocket terminal + GitLab OAuth + Chat API）過程中用到的工具與技巧。

---

新增需求

> 顯示的部份要顯示用戶問題，結果，最下面 Tool Calls 改成像 claude code 畫面有變動就輸出一點狀態，也有時間在變化
> 登入後也應該顯示用戶資訊（從 GitLab OAuth 拿到的 name/email），也能登出（清除 session cookie）

---

## 0. 環境說明

### 部署環境（QNAP NAS）

| 項目     | 值                                                   |
| -------- | ---------------------------------------------------- |
| 主機     | QNAP NAS，`cdrdla.myqnapcloud.com` / `172.17.28.122` |
| Port     | 10000（HTTP，無 TLS）                                |
| 容器名稱 | `perch`                                              |
| 部署目錄 | `/share/Container/doppelganger/`                     |

### GitLab 登入方式

- GitLab instance：`https://sauron.qnap.com`
- 登入方式：**Azure AD (M365)**（頁面上點選 "Azure AD (M365)" 按鈕，不用手動輸入帳密）
- SSO 若已登入 Azure AD，會自動完成 OAuth flow，無需人工介入
- 若 OAuth flow 有問題需要人工介入

### 瀏覽器自動化測試工具

使用 `chrome-cdp` skill（`/Users/dorowu/.claude/skills/chrome-cdp/scripts/cdp.mjs`）：

```bash
# 列出分頁（找 Perch 的 target ID）
node scripts/cdp.mjs list

# 常用指令
node scripts/cdp.mjs nav   <target> <url>      # 導航
node scripts/cdp.mjs shot  <target> [file]     # 截圖
node scripts/cdp.mjs snap  <target>            # accessibility tree
node scripts/cdp.mjs eval  <target> <expr>     # 執行 JS
node scripts/cdp.mjs click <target> <selector> # 點擊元素
node scripts/cdp.mjs evalraw <target> Network.getCookies '{"urls":["..."]}' # 查 cookies
```

**已知 Perch target ID**（每次重啟 Chrome 會變）：用 `list` 重新查，找「Perch」或對應 URL。

### 這套環境的特殊限制

- **Squid 透明 proxy**：QNAP 內部流量經過 Squid，WebSocket `Upgrade` header 被剝除 → Chat 改用 SSE
- **Chrome PNA**：`cdrdla.myqnapcloud.com` 解析到私有 IP `172.x.x.x`，WS 101 回應必須帶 `Access-Control-Allow-Private-Network: true`
- **/tmp 空間**：container 的 `/tmp` 只有 128MB tmpfs，大檔案用 `/share/ZFS1_DATA/homes/admin/` 中轉
- **SSH**：`cdrdla.myqnapcloud.com:22` 有時被防火牆擋，改用直連 IP `172.17.28.122`
- **OAuth origin 問題**：從 `localhost:10000` 開始的 OAuth flow，callback 會落在 `cdrdla:10000`，cookie domain 不符 → 用動態 redirect_uri 修正

### 本機 Debug 連線方式

這套環境比較特殊，必須按以下步驟才能正常測試：

**Step 1：SSH port forward（用戶手動開）**

SSH 必須用直連 IP `172.17.28.122`，不能用 `cdrdla.myqnapcloud.com`（22 port 可能被防火牆擋）：

```
ssh -L 10000:localhost:10000 admin@172.17.28.122
```

**Step 2：修改 /etc/hosts（用戶手動開）**

```
127.0.0.1  cdrdla.myqnapcloud.com
```

**Step 3：Chrome 用 `cdrdla.myqnapcloud.com:10000` 開啟**

因為 `/etc/hosts` 把 `cdrdla` 指向 `127.0.0.1`，實際流量會走 SSH port forward 到 QNAP。Chrome CDP 也是連這個分頁，所以 target URL 也是 `cdrdla.myqnapcloud.com:10000`。

**為什麼不直接用 127.0.0.1 或直連 IP：**

- 直接用 `127.0.0.1`：OAuth callback URL 會對不上 GitLab 設定的 redirect URI
- 直接用 `172.17.28.122`：Chrome PNA 政策會擋 WebSocket（公開 FQDN 解析到私有 IP）
- 用 `cdrdla` + `/etc/hosts` → `127.0.0.1`：瀏覽器認為是 localhost，PNA 不觸發，且 OAuth callback URL 一致

---

## 1. 容器除錯（Docker）

### 進入容器

```bash
docker exec -it <container> sh
docker exec -it <container> bash
```

### 確認環境變數

```bash
docker inspect <container> | jq '.[0].Config.Env'
# 或進入容器後
env | grep ANTHROPIC
env | grep CLAUDE
```

### 查看 log

```bash
docker logs <container>
docker logs <container> --tail 50 -f
```

### 部署二進位（不 rebuild image）

```bash
# 本機編譯
GOOS=linux GOARCH=amd64 go build -o perch_linux_amd64 .

# 傳到遠端（若 /tmp 空間不足，用有空間的目錄）
scp -P 22 perch_linux_amd64 user@host:/share/ZFS1_DATA/homes/admin/

# 複製進容器
docker cp /share/ZFS1_DATA/homes/admin/perch_linux_amd64 <container>:/app/perch

# 重啟容器（不 recreate，保留 volume）
docker restart <container>
```

### 確認 /tmp 空間

```bash
df -h /tmp
# 若滿了，用其他目錄 (e.g. /share/... on QNAP)
```

---

## 2. QNAP 部署流程

```bash
# 1. 編譯 Linux binary
GOOS=linux GOARCH=amd64 go build -o perch_linux_amd64 .

# 2. 前端 build（如有修改）
cd frontend && npm run build && cd ..

# 3. 傳到 QNAP（/tmp 只有 128MB，用 /share/... 代替）
scp -P 22 perch_linux_amd64 admin@172.17.28.122:/share/ZFS1_DATA/homes/admin/

# 4. 複製進容器並重啟
ssh admin@172.17.28.122
docker cp /share/ZFS1_DATA/homes/admin/perch_linux_amd64 perch:/app/perch
docker restart perch

# 5. 確認正常
docker logs perch --tail 20
curl -s http://localhost:10000/api/auth | jq .
```

### 若需要 recreate（改了 .env）

```bash
cd /share/ZFS1_DATA/homes/admin/deploy
docker compose down && docker compose up -d
```

---

## 3. 瀏覽器網路面板快速診斷

| 看什麼                  | 意義                                        |
| ----------------------- | ------------------------------------------- |
| WS status 200（非 101） | Proxy 剝除 Upgrade header                   |
| WS status 403           | Auth middleware 擋住                        |
| SSE 連線立刻關閉        | 後端沒有對應 session（先打 POST /api/chat） |
| 409 Conflict            | 已有 session 在執行                         |
| Set-Cookie domain 不對  | OAuth state cookie 設在錯誤 origin          |

### 快速用 curl 測試 SSE

```bash
curl -v -N \
  -H "Cookie: session_token=<token>" \
  http://localhost:10000/api/chat/stream
# 若看到 "data:" 開頭的行 → SSE 正常
```
