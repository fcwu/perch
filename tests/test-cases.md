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
- Discord tab **可輸入**（鍵盤輸入會寫入 PTY）

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

## T34 — Discord Open-Channel 模式：僅設 BOT_TOKEN 即可啟動

**目的**：確認移除 `DISCORD_CHANNEL_ID` 後，Bot 仍能正常啟動並連上 Discord。

**步驟**：
```bash
docker run --rm \
  -e DISCORD_BOT_TOKEN=<token> \
  -e AUTH_MODE=none \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  -p 8081:8080 \
  perch:local 2>&1 | head -10
```

**預期**：
- Log 出現 `Discord bot connected (per-channel PTY mode)` 或類似訊息
- 無 `DISCORD_CHANNEL_ID required` 或啟動失敗訊息
- 瀏覽器開啟 `http://localhost:8081` 正常顯示 terminal

---

## T35 — Discord Open-Channel：Public 頻道需 @mention

**目的**：未設 `DISCORD_CHANNEL_ID` 時，在 public Guild 頻道不 @mention Bot 應無回應；@mention 後才回應。

**前置條件**：container 以純 `DISCORD_BOT_TOKEN`（不帶 `DISCORD_CHANNEL_ID`）啟動，且測試頻道為 public（@everyone 可見）。

**步驟**：
1. 在 public 頻道直接傳送訊息（不 @mention）：「你好」
2. 觀察 30 秒，確認無任何 reaction 或回應
3. 在同一頻道傳送 @mention 訊息：`@Perch 你好`
4. 觀察 reaction 與回應

**預期**：
- 步驟 1–2：訊息無 👀 reaction，Bot 無任何回應（silent ignore）
- 步驟 3–4：訊息出現 👀 reaction；Claude 處理完後 Discord 收到 reply

---

## T36 — Discord Open-Channel：Private 頻道直接回應（不需 @mention）

**目的**：未設 `DISCORD_CHANNEL_ID` 時，在 private Guild 頻道（@everyone ViewChannel 被 deny）不需 @mention 即可直接對話。

**前置條件**：container 以純 `DISCORD_BOT_TOKEN` 啟動；測試頻道為 private（Server 設定中 @everyone 無法查看此頻道，但 Bot role 可以）。

**步驟**：
1. 在 private 頻道直接傳送訊息（不 @mention）：「你是誰？」
2. 觀察 reaction 與回應

**預期**：
- 訊息出現 👀 reaction（不需 @mention）
- Claude 處理後 Discord 收到 reply，內容回答問題
- PTY content 中可見訊息文字（無 `<@...>` 前綴）

---

## T37 — Discord Open-Channel：DM 直接回應（不需 @mention）

**目的**：使用者私訊 Bot 時，不需 @mention 即可直接對話。

**前置條件**：container 以純 `DISCORD_BOT_TOKEN` 啟動。

**步驟**：
1. 在 Discord 開啟與 Bot 的 DM
2. 直接傳送訊息：「今天日期是？」

**預期**：
- 訊息出現 👀 reaction
- Claude 回應後，Bot 在 DM 中回覆正確日期
- Web terminal 可看到對應的 Discord DM session tab（tab 名稱含 channel ID）

---

## T38 — Discord Backward Compat：設定 DISCORD_CHANNEL_ID 維持原行為

**目的**：同時設定 `DISCORD_BOT_TOKEN` 與 `DISCORD_CHANNEL_ID` 時，Bot 只回應指定頻道，其他頻道訊息被忽略（原有行為不變）。

**前置條件**：container 同時帶 `DISCORD_BOT_TOKEN` 與 `DISCORD_CHANNEL_ID`（指向特定頻道）。

**步驟**：
```bash
docker run --rm \
  -e DISCORD_BOT_TOKEN=<token> \
  -e DISCORD_CHANNEL_ID=<channel_id> \
  -e AUTH_MODE=none \
  -e TZ=Asia/Taipei \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v ~/.claude:/home/perchuser/.claude \
  -v ~/.claude.json:/home/perchuser/.claude.json \
  -v ./:/workspace \
  -p 8081:8080 \
  perch:local
```

1. 在**指定頻道**傳送訊息（不需 @mention）：「你好」
2. 在**另一個頻道**傳送訊息（不需 @mention）：「你好」

**預期**：
- 步驟 1：👀 出現，Claude 正常回應（原有行為）
- 步驟 2：無任何 reaction 或回應（channel filter 生效）

---

## T39 — Discord mention prefix 剝除（Public 頻道）

**目的**：Public 頻道 @mention 觸發時，Claude 收到的訊息不含 `<@BOT_ID>` 前綴，只有實際問題內容。

**前置條件**：container 以純 `DISCORD_BOT_TOKEN` 啟動（無 channel filter）。

**步驟**：
1. 在 public 頻道傳送：`@Perch 列出 /workspace 下的檔案`
2. 觀察 Web terminal 的 PTY 輸出

**預期**：
- PTY 中出現的文字為 `列出 /workspace 下的檔案`
- **不出現** `<@1234567890> 列出 /workspace 下的檔案`
- Claude 正常執行對應指令並回覆

---

## T40 — OpenCode Runtime 可啟動

**目的**：確認 `AGENT_RUNTIME=opencode` 時，Perch 主 PTY 會啟動 OpenCode，而不是 Claude。

**步驟**：
```bash
docker run --rm \
  -e AUTH_MODE=none \
  -e AGENT_RUNTIME=opencode \
  -e OPENCODE_ARGS="-q" \
  -e ANTHROPIC_API_KEY=<key> \
  -v /your/workspace:/workspace \
  -p 8081:8080 \
  perch:local
```

**預期**：
- perch 啟動成功，沒有 `invalid AGENT_RUNTIME` 或 `claude: not found`
- Web terminal 可連線
- 主 PTY 顯示 OpenCode 啟動畫面或 OpenCode 對應輸出，而非 Claude Code 畫面

---

## T41 — OpenCode Runtime：Discord 訊息可收到完成回覆

**目的**：確認 `AGENT_RUNTIME=opencode` 時，Discord 訊息仍會寫入 Discord session PTY，並在工作完成後收到回覆。

**前置條件**：
- 已設定 `DISCORD_BOT_TOKEN`
- container 以 `AGENT_RUNTIME=opencode` 啟動

**步驟**：
1. 在 Discord channel 傳送簡短訊息，例如「回我一個 hi」
2. 等待 OpenCode 完成

**預期**：
- 原始訊息出現 👀 reaction
- Bot 在同一 channel 以 reply 回覆最終結果
- 即使沒有 Claude hook 的 `⚙️ / ✅ / ❌` reaction，仍有 completion reply

---

## T42 — OpenCode Runtime：Discord 排程仍回到正確 Channel

**目的**：確認 `AGENT_RUNTIME=opencode` 時，`target: "discord:<channelID>"` 的排程仍會先發 header，再把完成結果送回相同頻道。

**前置條件**：
- 同 T41
- Discord 排程功能可用

**步驟**：
1. 建立一個一次性排程，目標為目前 Discord channel
2. 等待觸發

**預期**：
- 先出現 `📅 local schedule > ...` header
- 後續 completion reply 出現在同一 channel
- main terminal 沒有收到這次 Discord-targeted 排程輸出

---

## T43 — Web UI 對 Discord Session PTY 輸入

**目的**：確認 Web UI 使用者可以在 Discord session tab 中直接輸入文字，keystrokes 會寫入對應的 PTY。

**前置條件**：
- 同 T28（Discord bot 已啟動，Web UI 可見 Discord channel tab）

**步驟**：
1. 瀏覽器切換到 Discord channel tab
2. 在 terminal 中輸入任意文字（例如 `ls`）並按 Enter

**預期**：
- 鍵盤輸入出現在 web terminal 畫面
- PTY 實際執行輸入的指令，輸出顯示在同一畫面
- 同一 Discord channel 的輸出也同步反映（兩端共用同一 PTY）

**反向驗證**：`resize` 訊息仍正確觸發 PTY resize，不被當成 keystroke 寫入。

---

## T44 — Workspace Git Sync：功能停用（預設行為）

**目的**：確認未設定 `WORKSPACE_GIT_SYNC_ENABLED` 時，sync loop 不啟動，log 中無任何 `workspace_sync` 訊息。

**步驟**：
```bash
# workspace 為一個 git repo，但不設定 sync env var
docker run --rm \
  -e AUTH_MODE=none \
  -v /your/git-workspace:/workspace \
  -p 8080:8080 \
  perch:local 2>&1 | grep workspace_sync
```

**預期**：
- grep 無任何輸出（sync loop 未啟動）
- 啟動 log 中無 `workspace_sync:` 前綴的訊息

---

## T45 — Workspace Git Sync：HTTPS remote 正常 pull + push

**目的**：確認 HTTPS remote + token 設定後，定時 sync 能成功 pull 和 push。

**前置條件**：
- `/workspace` 是一個 git repo，remote 為 HTTPS（如 `https://github.com/user/repo.git`）
- Remote 上有新的 commit（手動在另一機器 push 一個測試 commit）

**步驟**：
```bash
docker run --rm \
  -e AUTH_MODE=none \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_SYNC_INTERVAL=15 \
  -e WORKSPACE_GIT_TOKEN=<your_github_pat> \
  -v /your/git-workspace:/workspace \
  -p 8080:8080 \
  perch:local 2>&1 | grep workspace_sync
```

**預期**：
- Log 出現 `workspace_sync: injecting git token for host github.com`（不含 token 值）
- Log 出現 `workspace_sync: git credential helper set to store`
- Log 出現 `workspace_sync: credential injection complete`
- 15 秒後出現 `workspace_sync: starting sync`
- Log 出現 `workspace_sync: pull output: ...`（含遠端 commit 資訊）
- Log 出現 `workspace_sync: push output: ...`
- Log 出現 `workspace_sync: sync complete`
- `git log` 在 workspace 中顯示遠端的新 commit 已拉下

**反向驗證（不設 token）**：
- Log 出現 `workspace_sync: no WORKSPACE_GIT_TOKEN set, skipping credential injection`
- pull/push 是否成功取決於系統 credential（可能失敗，但不 panic）

---

## T46 — Workspace Git Sync：rebase 衝突偵測與通知

**目的**：確認衝突時系統 abort rebase、記錄完整 log，並發 Discord 通知。

**前置條件**：
- `/workspace` 是一個 git repo，已設定 HTTPS remote + token
- 已設定 `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=<discord_channel_id>`
- 準備在遠端和本地修改同一行以製造衝突

**步驟**：
1. 在 workspace 的某個檔案某行寫入「version A」並 commit（但不 push）
2. 在遠端同一行寫入「version B」並 push（模擬另一使用者）
3. 啟動 perch 並等待 sync tick

```bash
docker run --rm \
  -e AUTH_MODE=none \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_SYNC_INTERVAL=15 \
  -e WORKSPACE_GIT_TOKEN=<token> \
  -e WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=<channel_id> \
  -e DISCORD_BOT_TOKEN=<bot_token> \
  -v /your/git-workspace:/workspace \
  -p 8080:8080 \
  perch:local 2>&1 | grep workspace_sync
```

**預期（log）**：
- `workspace_sync: starting sync`
- `workspace_sync: pull output: ...CONFLICT...` 或 `workspace_sync: rebase in progress, aborting`
- `workspace_sync: rebase abort output: <git output>`
- `workspace_sync: rebase abort succeeded`（或 `failed`）
- Discord 指定 channel 收到 `⚠️ git sync conflict` 訊息

**Debounce 驗證**：等待第二個 sync tick（15 秒後），確認 Discord 不再發第二則相同通知。

---

## T47 — Workspace Git Sync：push 失敗通知

**目的**：確認 push 被 remote reject 時，log 有完整錯誤輸出且 Discord 收到通知。

**前置條件**：同 T46，但不製造 rebase 衝突，改用 `--force-with-lease` 在遠端 force push，讓 workspace 的 push 被拒絕。

**步驟**：
1. 在遠端執行 `git push --force` 強制移動 branch HEAD
2. 啟動 perch 等待 sync tick

**預期**：
- `workspace_sync: push failed: ...` 出現在 log，含 git 的 stderr（`! [rejected]` 或 `Updates were rejected`）
- Discord channel 收到 `⚠️ git sync: git push failed` 通知

---

## T48 — Workspace Git Sync：SSH remote 忽略 token

**目的**：確認 SSH remote 下設定 `WORKSPACE_GIT_TOKEN` 不會影響行為，且 log 有 warning。

**前置條件**：`/workspace` 的 remote 為 `git@github.com:user/repo.git`（SSH）。

**步驟**：
```bash
docker run --rm \
  -e AUTH_MODE=none \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -e WORKSPACE_GIT_SYNC_INTERVAL=30 \
  -e WORKSPACE_GIT_TOKEN=sometoken \
  -v /your/git-workspace:/workspace \
  -p 8080:8080 \
  perch:local 2>&1 | grep workspace_sync
```

**預期**：
- Log 出現 `workspace_sync: git token ignored for SSH remote`
- **不出現** `workspace_sync: injecting git token`
- `~/.git-credentials` 未被修改（容器內可確認）

---

## T49 — Workspace Git Sync：非 git 目錄不啟動

**目的**：確認 `/workspace` 不是 git repo 時，sync loop 靜默跳過，不 crash。

**前置條件**：`/workspace` 存在但不含 `.git` 目錄。

**步驟**：
```bash
docker run --rm \
  -e AUTH_MODE=none \
  -e WORKSPACE_GIT_SYNC_ENABLED=true \
  -v /tmp/empty-dir:/workspace \
  -p 8080:8080 \
  perch:local 2>&1 | grep workspace_sync
```

**預期**：
- Log 出現 `workspace_sync: workspace is not a git repo, skipping sync`
- 無任何 `workspace_sync: starting sync` 訊息
- perch 正常啟動，無 crash

---

## 已知 Bug 清單

### T55 — Admin Live Sessions 只顯示使用工具的 session

Admin Live Sessions 透過 `/ws/admin` WebSocket 推送 `session_added` / `session_removed` 事件。
對於不呼叫任何工具的簡單查詢（例如 "say hi"），`ClaimUUID`（觸發 `session_added`）與
`NotifyHook("Stop")`（觸發 `session_removed`）在同一個 hook 呼叫序列內執行，間距 < 1ms，
瀏覽器 React 渲染來不及反映。

**影響範圍**：只有 no-tool 查詢在 Live Sessions 看不到；需要讀檔/搜尋的真實 KB 查詢（有 PreToolUse hook）可正常顯示，session 會維持數秒以上。

**建議驗證方式**：使用會觸發工具呼叫的查詢（如「列出 /workspace 的檔案」），觀察 Live Sessions 顯示 current tool 欄位即時更新。

### T52 — Chat UI textarea 送出後未清空（已知，低優先）

按下 Enter 或 Send 送出查詢後，textarea 清空依賴 `setQuery('')`；在某些 React 狀態時序下
可能不即時，使用者需手動刪除才能輸入下一個問題。

### T12 — mTLS generateClientP12 key mismatch

`AUTH_MODE=mtls` 下執行 `/bootstrap`，`tls.go` 的 `generateClientP12` 內部產生 RSA key pair 後，cert 與 private key 不匹配，導致 TLS handshake 失敗：

```
x509: provided PrivateKey doesn't match parent's PublicKey
```

影響範圍：T12 整個流程無法完成。其他認證模式（none、password）不受影響。



---

## T50 — GitLab OAuth：未登入 redirect

**目的**：確認 `/chat` 在沒有 cookie 時自動導向 GitLab OAuth。

**前置條件**：已設定 `GITLAB_URL`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`、`GITLAB_REDIRECT_URI`、`COOKIE_SECRET`。

**步驟**：
```bash
# 啟動（不帶 cookie）
curl -v http://localhost:8080/chat 2>&1 | grep "< HTTP\|Location:"
```

**預期**：
```
< HTTP/1.1 302 Found
Location: /auth/gitlab
```

**反向驗證（tampered cookie → 401）**：
```bash
curl -v -H "Cookie: perch_session=badpayload.badsig" \
  http://localhost:8080/chat 2>&1 | grep "< HTTP"
# 預期：HTTP/1.1 401 Unauthorized
```

**反向驗證（state mismatch → 400）**：
```bash
# callback 不帶 oauth_state cookie，或 state 不一致
curl -v "http://localhost:8080/auth/callback?code=abc&state=wrong" \
  2>&1 | grep "< HTTP"
# 預期：HTTP/1.1 400 Bad Request
```

---

## T51 — GitLab OAuth：完整登入流程

**目的**：確認 OAuth code exchange → 設置 signed cookie → redirect `/chat` 全流程正確。

**前置條件**：能連上 GitLab instance（`GITLAB_URL`）並已建立 OAuth Application。

**步驟**：
1. 瀏覽器開啟 `http://localhost:8080/chat`（未登入）
2. 應自動 redirect 到 GitLab 授權頁面
3. 在 GitLab 頁面授權
4. GitLab redirect 回 `/auth/callback?code=...&state=...`
5. 伺服器設置 `perch_session` cookie 後 redirect 到 `/chat`

**驗證 cookie 已設置**（DevTools → Application → Cookies）：
- Name: `perch_session`
- HttpOnly: ✓
- MaxAge: 28800（8 小時）
- Path: `/`

**驗證 cookie 內容有效**（Go unit test）：
```bash
go test -run TestGitLabAuthCallbackValidFlow ./...
```

**預期**：
- Step 4 回應為 HTTP 302 到 `/chat`
- cookie payload 解碼後含正確 `user_id`、`username`、`exp`

---

## T52 — Chat UI：查詢送出與 markdown 串流

**目的**：確認 POST `/api/chat` → WebSocket `/ws/chat` 全流程，以及 Tool Panel 即時更新。

**前置條件**：
- 有效 `perch_session` cookie（已完成 T51）
- `AGENT_RUNTIME=opencode`
- workspace 掛載知識庫，且 `.opencode/agents/as-query.md` 存在

**步驟（API 層驗證）**：
```bash
# 取得 cookie 後手動帶入（以 curl 模擬）
SESSION_COOKIE="perch_session=<從瀏覽器複製的值>"

# 1. 發送查詢
curl -v -X POST http://localhost:8080/api/chat \
  -H "Cookie: $SESSION_COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"query":"什麼是 HBS？"}' \
  2>&1 | grep "< HTTP\|user_id"
# 預期：HTTP/1.1 200 OK，body 含 {"user_id":"..."}

# 2. 確認 session 已建立（重複送出 → 409）
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/api/chat \
  -H "Cookie: $SESSION_COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"query":"再問一次"}'
# 預期：409（session 已在執行中）
```

**步驟（瀏覽器驗證）**：
1. 開啟 `http://localhost:8080/chat`，輸入問題，按 Enter
2. 觀察：
   - 輸入框 disabled、出現「⟳ Thinking…」
   - PTY 輸出以 markdown 逐步渲染
   - 點開 Tool calls 面板 → 看到工具名稱 + spinner
   - 工具完成 → spinner 變 ✓，顯示 elapsed ms
   - 完成後輸入框恢復可用

**預期 WebSocket 訊息順序**：
```
[binary]  PTY 輸出字節（逐步累積，markdown 渲染）
[text]    {"type":"tool_start","tool":"Read","input":...}
[binary]  更多 PTY 輸出
[text]    {"type":"tool_end","tool":"Read","elapsed_ms":42}
[text]    {"type":"done"}
```

---

## T53 — Chat UI：多使用者並行 Session 互不干擾

**目的**：確認不同 userID 的 session 輸出不會串流到錯誤的 WebSocket。

**前置條件**：兩組不同 GitLab 帳號各自登入，取得各自 `perch_session` cookie。

**步驟**：
```bash
# 使用者 A 送出慢查詢
curl -X POST http://localhost:8080/api/chat \
  -H "Cookie: perch_session=<USER_A_COOKIE>" \
  -H "Content-Type: application/json" \
  -d '{"query":"詳細介紹整個 HBS 架構"}'

# 使用者 B 送出另一個查詢
curl -X POST http://localhost:8080/api/chat \
  -H "Cookie: perch_session=<USER_B_COOKIE>" \
  -H "Content-Type: application/json" \
  -d '{"query":"Boxafe 是什麼？"}'

# 確認 A 的 session 仍在執行中（返回 409）
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/api/chat \
  -H "Cookie: perch_session=<USER_A_COOKIE>" \
  -H "Content-Type: application/json" \
  -d '{"query":"再問"}'
# 預期：409
```

**瀏覽器驗證**：
1. 開兩個不同瀏覽器視窗，各用不同帳號登入
2. 同時送出查詢
3. 確認各自的 markdown 輸出不混流

**預期**：
- A、B 各自有獨立 PTY session（`UserSessionManager` 以 userID 為 key）
- Hook 事件由 `session_uuid → userID` mapping 正確路由，互不干擾
- 任一使用者查詢完成後，另一使用者的 session 繼續正常執行

---

## T54 — Admin Login（Phase 2）

**目的**：確認 Admin token 登入流程正確，設置 `perch_admin` signed cookie。

**前置條件**：啟動 perch 時設定 `ADMIN_TOKEN=mysecret`。

**步驟**：
```bash
# 正確 token → 200 + cookie
curl -s -v -X POST http://localhost:8080/admin/login \
  -H "Content-Type: application/json" \
  -d '{"token":"mysecret"}' 2>&1 | grep "< HTTP\|Set-Cookie"
# 預期：HTTP 200，Set-Cookie: perch_admin=...

# 錯誤 token → 401
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/admin/login \
  -H "Content-Type: application/json" \
  -d '{"token":"wrong"}'
# 預期：401
```

**反向驗證（ADMIN_TOKEN 未設定 → 503）**：
```bash
# 啟動時不設 ADMIN_TOKEN
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/admin/history
# 預期：503
```

**自動化**：`go test -run TestAdminLogin ./...`

---

## T55 — Admin 即時監控（Phase 2）

**目的**：確認 `/ws/admin` WebSocket 正確推送即時 session 狀態。

**前置條件**：已完成 T54 取得 `perch_admin` cookie，GitLab OAuth 已設定。

**步驟**：
1. 瀏覽器開啟 `http://localhost:8080/admin`
2. 另一個瀏覽器以 GitLab 帳號登入並送出查詢
3. 觀察 Admin 頁面的 Live Sessions

**預期**：
- Admin 頁面連線後立即顯示目前 active sessions（含 snapshot 推送）
- 新查詢開始時出現新 row，顯示 username、query（截斷）、elapsed time、current tool
- tool 開始執行時，current tool 欄位即時更新（不重整頁面）
- 查詢結束時，該 row 從列表消失（`session_removed` 事件）

---

## T56 — Admin 歷史搜尋（Phase 2）

**目的**：確認 `/admin/history` API 與 UI 的搜尋與詳情展開功能。

**前置條件**：已有若干完成的查詢 session（執行過 T55）。

**步驟（API）**：
```bash
ADMIN_COOKIE="perch_admin=<from T54>"

# 列出所有 session
curl -s http://localhost:8080/admin/history \
  -H "Cookie: $ADMIN_COOKIE" | jq '.total, .sessions | length'

# 依使用者過濾
curl -s "http://localhost:8080/admin/history?user=alice" \
  -H "Cookie: $ADMIN_COOKIE" | jq '.sessions[].Username'

# 關鍵字搜尋
curl -s "http://localhost:8080/admin/history?q=kubernetes" \
  -H "Cookie: $ADMIN_COOKIE" | jq '.total'

# 取得單一 session 詳情
SESSION_ID=$(curl -s http://localhost:8080/admin/history -H "Cookie: $ADMIN_COOKIE" | jq -r '.sessions[0].ID')
curl -s "http://localhost:8080/admin/history/$SESSION_ID" \
  -H "Cookie: $ADMIN_COOKIE" | jq 'keys'
# 預期：含 "Response", "ToolEvents", "Query" 等欄位
```

**步驟（UI）**：
1. 瀏覽器開啟 `http://localhost:8080/admin/history`
2. 在搜尋欄輸入關鍵字，觀察列表即時過濾
3. 點擊某一行 → 展開詳情頁，可見完整 query、response、tool call 時序

**預期**：list API 不含 response 欄位；detail API 含完整 response 與 tool_events。

---

## T57 — Per-User Rate Limit 429 回應（Phase 3）

**目的**：確認超過 `RATE_LIMIT_RPM` 後回傳 429 及 retry_after_ms。

**前置條件**：啟動 perch 時設定 `RATE_LIMIT_RPM=2`，已取得 GitLab session cookie。

**步驟**：
```bash
SESSION_COOKIE="perch_session=<your_cookie>"

for i in 1 2 3; do
  CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/api/chat \
    -H "Cookie: $SESSION_COOKIE" \
    -H "Content-Type: application/json" \
    -d '{"query":"test '\"$i\"'"}')
  echo "Request $i: $CODE"
done
```

**預期**：
- Request 1, 2：HTTP 200（或 409 若 session 仍在執行中）
- Request 3：HTTP 429，body 含 `{"error":"rate limit exceeded","retry_after_ms":N}`

**自動化**：`go test -run TestUserRateLimiter ./...`

---

## T58 — JSON Log 格式驗證（Phase 3）

**目的**：確認 `LOG_FORMAT=json` 時，查詢生命週期事件以 JSON 格式輸出且含必要欄位。

**步驟**：
```bash
LOG_FORMAT=json GITLAB_URL=... ADMIN_TOKEN=x ./perch 2>&1 &

# 觸發一次查詢，觀察 log 輸出
curl -X POST http://localhost:8080/api/chat \
  -H "Cookie: $SESSION_COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"query":"test"}'
```

**預期**：Log 輸出為 newline-delimited JSON，包含：
```json
{"time":"...","level":"INFO","msg":"query_start","user_id":"...","session_id":"...","query":"test"}
{"time":"...","level":"INFO","msg":"tool_start","session_id":"...","tool":"Read"}
{"time":"...","level":"INFO","msg":"query_done","session_id":"...","duration_ms":N,"status":"done"}
```

**反向驗證**：不設 `LOG_FORMAT`（預設 text），log 輸出為 `time=... level=INFO msg=...` 格式。

**自動化**：`go test -run TestLogger ./...`

---

## T59 — Analytics API 回傳正確統計（Phase 3）

**目的**：確認 `GET /admin/analytics` 回傳正確的 per-user 統計與 top tools 排行。

**前置條件**：已有若干完成的查詢 session（含 tool events）。

**步驟**：
```bash
ADMIN_COOKIE="perch_admin=<from T54>"

# 本週統計
FROM=$(date -d '7 days ago' +%s000 2>/dev/null || date -v-7d +%s000)
TO=$(date +%s000)
curl -s "http://localhost:8080/admin/analytics?from=$FROM&to=$TO" \
  -H "Cookie: $ADMIN_COOKIE" | jq '.'
```

**預期**：
```json
{
  "users": [{"username":"alice","query_count":N,"avg_duration_ms":M},...],
  "top_tools": [{"tool":"read","count":N},...],
  "total_queries": N,
  "total_duration_ms": M
}
```
- users 依 query_count 降冪排列
- top_tools 依 count 降冪排列（最多 10 個）
- 無資料時回傳空陣列，total_queries=0

**自動化**：`go test -run TestGetUsageStats ./...`

---

## T60 — GET /admin/login 應回傳 SPA HTML

**目的**：確認瀏覽器直接輸入 `/admin/login` URL 時，伺服器回傳 `index.html` 而非 "method not allowed"。

**背景**：`handleLogin` 只處理 POST；若 GET 請求命中同一 handler 會回 405。
此 bug 曾在初次 browser 測試時發現，需確保路由層對 GET 正確 fallback 到 SPA。

**步驟**：
```bash
# GET 請求應回傳 HTML（SPA entry point），不是純文字錯誤
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/admin/login
# 預期：200

curl -s http://localhost:8080/admin/login | head -1
# 預期：<!doctype html>
```

**瀏覽器驗證**：
1. 在 Chrome 網址列輸入 `http://<host>/admin/login` 並按 Enter
2. 確認頁面渲染出「Admin Login」表單（有 token 輸入框和 Login 按鈕）
3. 不應出現「method not allowed」或任何純文字錯誤

**反向驗證**：POST 仍走 login handler
```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/admin/login \
  -H "Content-Type: application/json" -d '{"token":"correct"}'
# 預期：200（正確 token）
```

---

## T61 — Admin Tab 切換應為 Client-Side Routing

**目的**：確認 Live / History / Analytics tab 切換時，不觸發整頁重載；URL 更新但頁面不重新向 server 請求 HTML。

**背景**：若 tab 使用 `window.location.href`，瀏覽器會發 GET 到 `/admin/history`，
server 回 JSON（`{"sessions":null,"total":0}`），破壞 SPA 體驗。

**步驟（瀏覽器驗證）**：
1. 登入 `/admin/login` 取得 `perch_admin` cookie
2. 開啟 DevTools → Network 面板，勾選 「Preserve log」
3. 點擊「History」tab → 確認 **網址列變成 `/admin/history`** 但無 document 類型的 network request
4. 點擊「Analytics」tab → 確認 **網址列變成 `/admin/analytics`** 但無整頁 reload
5. 點擊「Live」tab → 回 `/admin`，頁面仍是 SPA

**預期**：
- Tab 切換只觸發 `fetch()` API call（`/admin/history`、`/admin/analytics`），不觸發整頁 navigation
- Network 面板中無 type=`document` 的請求（除最初進入頁面那次）
- URL 更新後，重新整理頁面仍可正確載入對應 tab

**反向驗證（不應出現的現象）**：
```bash
# 若 tab 用 window.location.href，直接 GET /admin/history 會拿到 JSON
curl -s http://localhost:8080/admin/history -H "Cookie: perch_admin=<token>" | head -1
# 正確行為下（有 cookie）仍回 JSON（API endpoint），不影響 SPA
# 但瀏覽器 tab 切換不應走這條路徑取 HTML
```

---

## T62 — Analytics API JOIN Query 不報 Ambiguous Column Error

**目的**：確認 `GET /admin/analytics` 在有資料時不回 500 Internal Server Error。

**背景**：`GetUsageStats` 的 tool_events JOIN query 中，`started_at` 在 `query_sessions` 和 `tool_events` 兩個 table 都存在，
若 WHERE 子句未加 table alias 前綴，SQLite 報 ambiguous column → 500。

**步驟**：
```bash
ADMIN_COOKIE="perch_admin=<from T54>"
FROM=$(($(date +%s) - 604800))000
TO=$(date +%s)000

curl -s -o /dev/null -w "%{http_code}" \
  "http://localhost:8080/admin/analytics?from=$FROM&to=$TO" \
  -H "Cookie: $ADMIN_COOKIE"
# 預期：200（不是 500）

curl -s "http://localhost:8080/admin/analytics?from=$FROM&to=$TO" \
  -H "Cookie: $ADMIN_COOKIE" | python3 -m json.tool
# 預期：合法 JSON，含 users、top_tools、total_queries、total_duration_ms
```

**反向驗證（無資料）**：
```bash
# from/to 超出資料範圍時，回傳空陣列而非 500
curl -s "http://localhost:8080/admin/analytics?from=0&to=1" \
  -H "Cookie: $ADMIN_COOKIE"
# 預期：{"users":[],"top_tools":[],"total_queries":0,"total_duration_ms":0}
```

**自動化**：`go test -run TestGetUsageStats ./...`
