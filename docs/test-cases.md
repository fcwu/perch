# Perch 測試案例

> 更新日期：2026-04-10

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

# 下載 client.p12（第一次）
curl -sk https://localhost:8443/bootstrap -o client.p12
echo "Exit: $?"

# 再次嘗試（應失效）
curl -sk https://localhost:8443/bootstrap -o /dev/null -w "%{http_code}"
```

**預期**：
- 第一次：下載成功（200），回傳 `client.p12`
- 第二次：HTTP 404 或 410（端點已失效）

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

## 已知 Bug 清單

無。
