# Perch 測試案例

> 更新日期：2026-04-12（rev 4）

---

## T01 — 啟動（none 模式）

**目的**：確認 `AUTH_MODE=none` 以 plain HTTP 啟動，瀏覽器可連上。

**步驟**：
```bash
AUTH_MODE=none LISTEN_ADDR=:8080 ./perch
curl -s http://localhost:8080 | head -3
```

**預期**：回傳 HTML（`<!doctype html>`），HTTP 200。

**反向驗證**：HTTPS 應失敗。
```bash
curl -v https://localhost:8080
# 預期：SSL handshake error（非 TLS server）
```

---

## T02 — 前端載入

**目的**：xterm.js terminal UI 正常渲染。

**步驟**：瀏覽器開啟 `http://localhost:8080`。

**預期**：
- 黑色 terminal 畫面出現
- 底部有虛擬鍵盤列
- status bar 顯示使用者名稱與工作目錄

---

## T03 — Terminal 輸出（PTY 串流）

**目的**：Claude Code 的輸出正常出現在 xterm.js。

**步驟**：觀察 T02 開啟後的 terminal 畫面。

**預期**：Claude Code 啟動畫面（Welcome 訊息）出現在 terminal 中。

---

## T04 — Terminal 輸入

**目的**：使用者輸入的文字能送入 PTY。

**步驟**：點擊 terminal 畫面，輸入任意文字後按 Enter。

**預期**：文字出現在 terminal 提示符號後，Claude Code 收到並處理。

---

## T05 — 排程器 列出

**目的**：確認排程資料以 JSONL 格式儲存，可直接讀取。

**步驟**：在 terminal 中詢問 Claude：「目前有哪些排程？」

**預期**：
- Claude 執行 `cat .perch/schedules.jsonl`（或顯示 no schedules）
- 回傳的每一行均為合法 JSON object，含 `id`、`hour`、`minute`、`message` 欄位

---

## T06 — 排程器 新增

**目的**：確認透過自然語言可新增排程，Perch 立即偵測並載入。

**步驟**：在 terminal 中告訴 Claude：「每天早上 9 點提醒我喝水，重複執行」

**預期**：
- Claude 使用 `local-schedule` skill，append 一行 JSON 到 `.perch/schedules.jsonl`
- Perch log 出現 `schedule added id=... hour=9 minute=0 ...`
- `cat .perch/schedules.jsonl` 可看到新 job 含 `id` 欄位

---

## T07 — 排程器 刪除

**目的**：確認透過自然語言可刪除排程，Perch 立即偵測並移除。

**步驟**：在 terminal 中告訴 Claude：「刪除剛才那個喝水提醒」

**預期**：
- Claude 找到對應 `id`，從 `.perch/schedules.jsonl` 移除該行
- Perch log 出現 `schedule deleted id=...`
- `cat .perch/schedules.jsonl` 該 job 消失

---

## T08 — 虛擬鍵盤

**目的**：虛擬鍵盤行為符合裝置類型預期，按鈕能正確送出按鍵序列。

**步驟（電腦瀏覽器）**：
1. 開啟 `http://localhost:8080`
2. 觀察畫面右下角

**預期（電腦）**：
- 預設顯示 ⌨ 浮動按鈕（鍵盤列已收合）
- 點擊 ⌨ → 底部展開虛擬鍵盤列
- 鍵盤列顯示：Esc、↑、↓、←、→、▼

**步驟（手機瀏覽器）**：
1. 手機開啟 `http://<server-ip>:8080`
2. 觀察畫面底部

**預期（手機）**：
- 預設展開顯示虛擬鍵盤列（無需點擊 ⌨）
- 鍵盤列顯示：Esc、↑、↓、←、→、▼
- 點擊 ▼ → 鍵盤收合，改顯示 ⌨
- 點擊各方向鍵 → terminal 對應移動游標／歷史指令
- 點擊 Esc → terminal 收到 Escape

**步驟（手機原生鍵盤彈出）**：
1. 手機點擊 terminal 觸發原生鍵盤彈出
2. 觀察 terminal 重繪

**預期**：terminal 縮小至剩餘可視區域，底部游標行保持可見（不被鍵盤遮住）

---

## T09 — Rate Limit

**目的**：`/login` 與 `/bootstrap` 端點有限速保護。

**步驟**：
```bash
for i in $(seq 1 8); do
  CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/login \
    -H "Content-Type: application/json" -d '{"password":"x"}')
  echo "Request $i: $CODE"
done
```

**預期**：前 5 次回傳非 429（404 or 401），第 6 次起回傳 **429 Too Many Requests**。

---

## T10 — 密碼模式

**目的**：`AUTH_MODE=password` 登入流程正確。

**步驟**：
```bash
AUTH_MODE=password AUTH_PASSWORD=testpass LISTEN_ADDR=:8081 ./perch &

# 正確密碼
curl -s -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"password":"testpass"}' -v

# 錯誤密碼
curl -s -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"password":"wrong"}' -v
```

**預期**：
- 正確密碼：HTTP 204，`Set-Cookie: session=...`
- 錯誤密碼：HTTP 401

**反向驗證**：未帶 cookie 直接存取 `/` 應收到 HTTP 401。

---

## T11 — 多連線 Framebuffer Replay

**目的**：新連線的 tab 能看到既有 PTY session 的完整畫面。

**步驟**：
1. 開啟 Tab A，等待 Claude Code 啟動輸出出現
2. 開啟 Tab B，連上同一 `http://localhost:8080`
3. 觀察 Tab B 是否顯示與 Tab A 相同內容

**預期**：Tab B 立即顯示與 Tab A 相同的 terminal 畫面（framebuffer replay，最多 1MB）。

---

## T12 — mTLS Bootstrap 流程

**目的**：`AUTH_MODE=mtls` 首次設定流程正確，`/bootstrap` 端點一次性。

**步驟**：
```bash
AUTH_MODE=mtls LISTEN_ADDR=:8443 ./perch &

# 下載 client.p12（第一次，不帶 client cert）
curl -sk https://localhost:8443/bootstrap -o client.p12
echo "Exit: $?"

# 再次嘗試（應失效）
curl -sk https://localhost:8443/bootstrap -o /dev/null -w "%{http_code}"
```

**預期**：
- 第一次：**不需帶 client certificate** 即可存取，下載成功（200），回傳 `client.p12`
- 第二次：HTTP 410（端點已失效，one-time only）
- 其他任何路徑在無 client cert 時 → **自動 302 跳轉** 到 `/bootstrap`

---

## T13 — Working Directory Mount（Docker）

**目的**：確認 Claude Code 在 container 內能存取掛載的 workspace。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/home/perchuser/.claude \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

在 terminal 中執行：
```
ls /workspace
```

**預期**：`/workspace` 目錄列出主機上 `/your/workspace` 的內容。

---

## T14 — 多連線雙向輸入同步

**目的**：任一 tab 輸入的文字，所有 tab 都能即時看到輸出；任一 tab 都能控制同一個 PTY。

**步驟**：
1. 開啟 Tab A 與 Tab B，兩個都連上 `http://localhost:8080`
2. 在 Tab A 點擊 terminal，輸入指令並送出
3. 觀察 Tab B 是否出現同樣的輸出
4. 在 Tab B 輸入另一個指令並送出
5. 觀察 Tab A 是否出現同樣的輸出

**預期**：
- Tab A 輸入 → Tab B 即時看到輸出
- Tab B 輸入 → Tab A 即時看到輸出
- 兩個 tab 始終呈現相同的 terminal 狀態

---

## T15 — 掛載 ~/.claude → Claude Code 已登入

**目的**：確認 `-v ~/.claude:/home/perchuser/.claude` 能讓 Claude Code 直接使用主機的登入憑證，不需重新登入。

**前置條件**：主機的 `~/.claude` 目錄中已有有效的 Claude Code 憑證（執行過 `claude` 並完成登入）。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/home/perchuser/.claude \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

瀏覽器開啟 `http://localhost:8080`，觀察 terminal 輸出。

**預期**：
- Claude Code 啟動後直接顯示 Welcome 訊息或 prompt，**不出現登入提示**
- 無 `Please log in` 或 OAuth 相關訊息

---

## T16 — 未掛載 ~/.claude → Claude Code 未登入

**目的**：確認未掛載 `~/.claude` 時，Claude Code 正確顯示登入提示，不使用任何殘留憑證。

**步驟**：
```bash
docker run -d \
  -p 8082:8082 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8082 \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

瀏覽器開啟 `http://localhost:8082`，觀察 terminal 輸出。

**預期**：
- Claude Code 啟動後出現登入提示（例如 `Please log in` 或引導使用者執行 `claude` 登入指令）
- **不自動進入** Ready 狀態

**反向驗證**：掛載 `-v ~/.claude:/home/perchuser/.claude` 後重建 container，應進入 T15 的已登入狀態。

---

## T17 — 首次開啟 Web UI Terminal 填滿畫面

**目的**：確認瀏覽器首次載入時，xterm.js terminal 完整填滿可視區域，無空白邊緣。

**步驟**：
1. 啟動 Perch（任何認證模式均可）
2. 開啟一個**全新**瀏覽器分頁，直接輸入 `http://localhost:8080`
3. 觀察頁面載入後的 terminal 畫面

**預期**：
- Terminal 黑色區域填滿整個 viewport（上下左右無明顯空白）
- 無需手動縮放或重整頁面
- xterm.js 的字元欄位數（cols）與列數（rows）正確對應視窗大小，送出的 resize 訊息已反映正確尺寸

**反向驗證**：調整瀏覽器視窗大小後，terminal 應自動重新 fit，不出現黑色空白條。

---

## T18 — Discord 訊息寫入 PTY

**目的**：確認 Discord channel 的訊息能正確寫入 PTY，Claude 收到並回應。

**前置條件**：已完成 Discord Bot 設定，取得 `DISCORD_BOT_TOKEN` 和 `DISCORD_CHANNEL_ID`。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -e DISCORD_BOT_TOKEN=your_token \
  -e DISCORD_CHANNEL_ID=your_channel_id \
  -v ~/.claude:/home/perchuser/.claude \
  ghcr.io/fcwu/perch:latest
```

在 Discord channel 傳送訊息：「你好，今天幾號？」

**預期**：
- 訊息傳入後，Discord 訊息上出現 👀 reaction
- 瀏覽器 terminal 可看到該訊息文字出現在 PTY

---

## T19 — Discord Hook Reaction 狀態機

**目的**：確認 Claude 執行工具期間，emoji reaction 正確反映執行狀態。

**前置條件**：同 T18，且 Claude Code Hooks 已啟用（settings.json bake 進 image）。

**步驟**：
1. 在 Discord 傳送會觸發工具的指令，例如：「列出 /workspace 下的所有檔案」
2. 觀察該訊息上的 reaction 變化

**預期**：
- 傳送後 → 👀 出現
- Claude 呼叫 Bash 工具時 → ⚙️ 出現
- 工具執行完成 → ✅ 出現，⚙️ 消失
- Claude 回應完畢 → 💬 出現，👀 消失，Discord 收到 reply 訊息

---

## T20 — Password 模式：所有端點受保護（unit test）

> **自動化**：`go test` → `TestAuthPasswordBlocksAllEndpoints`

**目的**：確認 password 模式下，所有受保護端點在無 session cookie 時回傳 401。

**涵蓋路徑**：`/`、`/ws`、`/input`、`/schedule`、`/schedule/:id`

**預期**：無 session cookie → HTTP 401。

---

## T21 — Password 模式：/login 與 /bootstrap 不需 session（unit test）

> **自動化**：`go test` → `TestAuthPasswordBypassEndpoints`

**目的**：確認登入與 bootstrap 端點在無 session 時可直接到達（否則使用者無法完成登入）。

**預期**：`/login`、`/bootstrap` → 非 401。

---

## T22 — Password 模式：Session Cookie 無 Secure Flag（unit test）

> **自動化**：`go test` → `TestAuthPasswordSessionCookieNotSecure`

**目的**：password 模式使用 plain HTTP，`Secure` flag 會導致瀏覽器不回送 cookie，使登入後所有請求仍被擋下。

**預期**：`/login` 回傳的 `Set-Cookie` 中，session cookie 不得帶 `Secure` 屬性。

---

## T23 — mTLS 模式：無 Client Cert 自動跳轉 /bootstrap（unit test）

> **自動化**：`go test` → `TestAuthMTLSRedirectsWithoutClientCert`

**目的**：確認 mtls 模式下，沒有 client certificate 的請求被自動跳轉到 `/bootstrap`，讓使用者能完成首次設定。

**涵蓋路徑**：`/`、`/ws`、`/input`、`/schedule`

**預期**：無 client cert → HTTP 302，`Location: /bootstrap`。

---

## T24 — mTLS 模式：/bootstrap 不需 Client Cert（unit test）

> **自動化**：`go test` → `TestAuthMTLSBootstrapAccessibleWithoutClientCert`

**目的**：`/bootstrap` 是取得 client cert 的唯一管道，若也要求 client cert 則永遠無法 bootstrap（雞生蛋問題）。TLS 層設為 `RequestClientCert`（optional），application layer 對 `/bootstrap` 不檢查憑證。

**預期**：`/bootstrap` 在無 client cert 時 → 非 401（可到達 handler）。

---

---

## T25 — 多行 URL 偵測與點擊

**目的**：terminal 中超過一行的 URL 能正確偵測為可點擊連結。

**步驟**：
1. 在 terminal 中輸入或觀察一個長 URL（長到折行），例如：
   ```
   https://example.com/very/long/path/that/wraps/across/two/lines/abc123
   ```
2. 滑鼠 hover 該 URL

**預期**：
- URL 帶底線且顯示 pointer 游標（即使跨多行）
- 點擊後在新 tab 開啟正確的完整 URL
- 不因換行切斷 URL

---

## T26 — Entrypoint Skill 合併

**目的**：掛載自己的 `~/.claude` 後，perch 內建 skill 仍可用，且不修改 `~/.claude/settings.json`。

**前置條件**：有一個 `local-schedule` skill 不在主機 `~/.claude/skills/` 中（可確認 `ls ~/.claude/skills/` 不含此 skill）。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/home/perchuser/.claude \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

**預期**：
- `/your/workspace/.claude/skills/local-schedule/` 目錄存在（skill 已複製進 workspace）
- 主機的 `~/.claude/settings.json` 內容**未被修改**
- 在 terminal 中告訴 Claude 設定排程，Claude 能使用 `local-schedule` skill

**反向驗證（IM 未設定時）**：
- 不設 `DISCORD_BOT_TOKEN` / `TELEGRAM_BOT_TOKEN`
- 重啟後確認 `/your/workspace/.claude/settings.json` 不存在（hooks 未被寫入）

---

## T27 — 排程資料存入 workspace 隱藏目錄

**目的**：排程資料儲存在 `workspace/.perch/schedules.jsonl`，不影響工作區內容，重啟後不遺失。

**步驟**：
1. 啟動 container，透過自然語言告訴 Claude 設定一個排程
2. 確認檔案位置：
   ```bash
   ls /your/workspace/.perch/
   cat /your/workspace/.perch/schedules.jsonl
   ```
3. 重啟 container
4. 再次確認排程仍存在

**預期**：
- 第一次：`.perch/schedules.jsonl` 存在，內含剛設定的 job
- 重啟後：同一個 job 仍在

---

## T28 — Discord Session Web Viewer（分頁顯示）

**目的**：確認 Web UI 可以在分頁中即時觀看 Discord channel 的 PTY 輸出（唯讀）。

**前置條件**：同 T18，Discord Bot 已連線，`DISCORD_CHANNEL_ID` 已設定。

**步驟**：
1. 啟動 container（含 Discord 環境變數）
2. 瀏覽器開啟 `http://localhost:8080`
3. 觀察頁面上方是否出現 tab 列
4. 點擊 Discord channel tab

**預期**：
- Tab 列出現，顯示「discord:<channel_id>」tab 與原本的主 terminal tab
- 點擊 Discord tab → terminal 畫面切換為該 Discord channel 的 PTY 輸出
- 從 Discord 傳送訊息後，web viewer 可看到 Claude 回應的輸出
- Discord tab **無法輸入**（鍵盤輸入不寫入 PTY）

**反向驗證**：未設定 Discord 環境變數時，tab 列不顯示（只有主 terminal）。

---

## T29 — Discord Session PTY Resize

**目的**：確認在 web viewer 中調整視窗大小時，Discord session 的 PTY 也同步 resize。

**前置條件**：同 T28，正在檢視 Discord channel tab。

**步驟**：
1. 切換到 Discord channel tab
2. 調整瀏覽器視窗大小
3. 在 Discord 傳送一個指令觸發長輸出

**預期**：
- `GET /sessions` 端點回傳 JSON 陣列，含 `channel_id` 與 `session_uuid`
- Terminal 畫面填滿調整後的視窗，無空白邊緣
- PTY 的 cols/rows 已隨視窗正確更新（輸出換行位置正確）

---

## T30 — 非 root 容器（PUID/PGID）

**目的**：確認容器以指定的使用者身份執行，workspace 檔案不被 root 所有，且 `bypassPermissions` 正常運作。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v /your/workspace:/workspace \
  ghcr.io/fcwu/perch:latest
```

容器啟動後：
```bash
# 確認行程執行身份
docker exec <container> id

# 確認 workspace 新建檔案的擁有者
docker exec <container> touch /workspace/test-owner.txt
ls -la /your/workspace/test-owner.txt
```

在 web terminal 中讓 Claude 執行一個需要 bypassPermissions 的指令（例如讀取/寫入檔案）。

**預期**：
- `id` 回傳的 uid/gid 與主機 `$(id -u)` / `$(id -g)` 一致，**不是 0（root）**
- `test-owner.txt` 的擁有者為主機使用者，而非 root
- Claude 不出現 `--dangerously-skip-permissions cannot be used with root` 錯誤
- Claude 能正常執行工具操作（bypassPermissions 有效）

**反向驗證**：不帶 PUID/PGID，重啟後 `id` 應顯示 uid=1000（預設值），workspace 檔案仍非 root。

---

---

## T31 — Discord 排程觸發回傳到正確 Channel

**目的**：確認 `target: "discord:<channelID>"` 的排程 job 在觸發時，Claude 的回應正確出現在 Discord channel，而非 main terminal。

**前置條件**：同 T18，Discord Bot 已連線。

**步驟**：
1. 在 Discord channel 傳送訊息，請 Claude 建立一個一次性排程（`repeat: false`），時間設為當前時間後 1 分鐘，例如：「一分鐘後說一句鼓勵的話，只說一次」
2. 等待觸發時間到來

**預期**：
- Discord channel 出現 `📅 local schedule > 請說一句簡短的鼓勵`
- Claude 回應後，Discord 收到 reply 訊息，thread 到上面那則 header
- Main terminal tab 無任何新輸出（訊息沒有跑到主 PTY）
- `repeat: false` 的 job 觸發後從 `GET /schedule` 消失

**反向驗證**：若 `target` 欄位為空，訊息應出現在 main terminal，Discord 無反應。

---

## T32 — Discord 排程 Header 訊息格式

**目的**：確認排程觸發時，Discord 先顯示 header 訊息，Claude 回覆以 thread reply 形式附在 header 下方。

**前置條件**：同 T31，排程已設定並等待觸發。

**步驟**：觀察 T31 觸發後 Discord channel 的訊息結構。

**預期**：
- Header 訊息格式：`📅 local schedule > {排程訊息內容}`
- Claude 的回覆：以 Discord reply（reply reference）附在 header 訊息下方
- 可清楚辨識此次輸出是由排程觸發，而非使用者手動輸入

---

## T33 — Build Time 顯示在啟動 Log

**目的**：確認 perch 啟動時 log 中包含 build time，方便確認部署的版本。

**步驟**：
```bash
docker run --rm \
  -e AUTH_MODE=none \
  ghcr.io/fcwu/perch:latest 2>&1 | head -5
```

**預期**：啟動 log 中包含 `built=` 欄位，值為 ISO 8601 格式的 UTC 時間，例如：
```
time=... level=INFO msg="perch listening" addr=:8080 auth=none built=2026-04-12T10:30:00Z
```

**反向驗證**：本機直接 `go build` 後執行（不帶 ldflags），`built=unknown` 應出現在 log 中。

---

## 已知 Bug 清單

### T12 — mTLS generateClientP12 key mismatch

`AUTH_MODE=mtls` 下執行 `/bootstrap`，`tls.go` 的 `generateClientP12` 內部產生 RSA key pair 後，cert 與 private key 不匹配，導致 TLS handshake 失敗：

```
x509: provided PrivateKey doesn't match parent's PublicKey
```

影響範圍：T12 整個流程無法完成。其他認證模式（none、password）不受影響。
