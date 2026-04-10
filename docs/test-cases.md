# Perch 測試案例

> 更新日期：2026-04-11

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

## T05 — 排程器 GET

**目的**：`GET /schedule` 回傳正確 JSON 格式。

**步驟**：
```bash
curl -s http://localhost:8080/schedule
```

**預期**：回傳 JSON 陣列（空陣列或現有 jobs），HTTP 200。

---

## T06 — 排程器 POST

**目的**：`POST /schedule` 新增 job 並回傳 ID。

**步驟**：
```bash
curl -s -X POST http://localhost:8080/schedule \
  -H "Content-Type: application/json" \
  -d '{"hour": 9, "minute": 0, "message": "test job", "repeat": true}'
```

**預期**：HTTP 201，`GET /schedule` 後可看到新 job 含 `id` 欄位。

---

## T07 — 排程器 DELETE

**目的**：`DELETE /schedule/:id` 刪除指定 job。

**步驟**：
```bash
curl -s -X DELETE http://localhost:8080/schedule/<id>
curl -s http://localhost:8080/schedule
```

**預期**：DELETE 回傳 HTTP 204，GET 後該 job 消失。

---

## T08 — 手機虛擬鍵盤

**目的**：虛擬鍵盤按鈕可見且能送出特殊按鍵。

**步驟**：觀察瀏覽器底部虛擬鍵盤列；點擊各按鈕。

**預期**：
- 顯示：Tab、Ctrl+C、Ctrl+D、Ctrl+Z、Esc、↑↓←→、▼
- 點擊 Tab → terminal 觸發補全
- 點擊 Ctrl+C → 中斷目前輸入

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
  -v ~/.claude:/root/.claude \
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

**目的**：確認 `-v ~/.claude:/root/.claude` 能讓 Claude Code 直接使用主機的登入憑證，不需重新登入。

**前置條件**：主機的 `~/.claude` 目錄中已有有效的 Claude Code 憑證（執行過 `claude` 並完成登入）。

**步驟**：
```bash
docker run -d \
  -p 8080:8080 \
  -e AUTH_MODE=none \
  -e LISTEN_ADDR=:8080 \
  -v ~/.claude:/root/.claude \
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

**反向驗證**：掛載 `-v ~/.claude:/root/.claude` 後重建 container，應進入 T15 的已登入狀態。

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
  -v ~/.claude:/root/.claude \
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

## 已知 Bug 清單

無。
