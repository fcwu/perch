# Chat / Discord 圖片上傳 測試案例

> 功能：chat-discord-image-upload
> 涵蓋範圍：Web Chat 上傳（file picker / drag-drop / paste）、Discord attachment 處理、server-side 限制、history placeholder、fetch failure fallback
> 撰寫日期：2026-05-01
> 相關 openspec：`chat-api-acp`、`discord-acp-session`

---

## 共通前置

- Perch 以預設模式啟動（`PERCH_MODE=single` 或 `multi` 皆可）
- chat-API 路徑：`POST /api/chat`，Discord 路徑：bot 在指定 channel 上線
- 預設限制：MIME `image/png|jpeg|gif|webp`、單檔 10MB、單次 4 張
- 測試圖片用容器內已存在的小圖（無也可以用 `printf` 產一張 1×1 PNG，base64 約 100 byte）

---

## E2E-curl

### CU01 — 純文字 + 1 張 PNG → ACP 收到含 image 的 prompt

**層級**：E2E-curl + WS subscriber

**Given** Perch 已啟動，使用者已登入
**When** 客戶端送出：
```bash
B64=$(base64 -w0 < tests/fixtures/tiny.png)
curl -sS -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"what's in this image?\",\"new_conversation\":true,\"attachments\":[{\"filename\":\"tiny.png\",\"mime_type\":\"image/png\",\"data_base64\":\"$B64\"}]}"
```

**Then**
- HTTP 200，回傳 `{"user_id":"...","conversation_id":"..."}`
- log 出現 `ACP chat: attachments accepted count=1`
- `/api/chat/stream` SSE 收到完整 lifecycle，最後 `done` event
- `GET /api/management/history?limit=1` 詳情頁 query 欄位開頭為 `[image:tiny.png] `（placeholder 格式）

---

### CU02 — Server-side 限制：超過 4 張 / 超過 10MB / 非允許 MIME 全拒絕

**層級**：E2E-curl

**Given** 預設限制
**When**
```bash
# (a) 5 張：超過上限
curl -sS -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"q\",\"attachments\":[<5 個 png>]}"

# (b) 11MB：超過 size
curl -sS -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"q\",\"attachments\":[<11MB png>]}"

# (c) SVG：MIME 不在白名單
curl -sS -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"q\",\"attachments\":[{\"filename\":\"x.svg\",\"mime_type\":\"image/svg+xml\",\"data_base64\":\"...\"}]}"

# (d) Magic mismatch：claim png 但 bytes 是 jpeg
curl -sS -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"q\",\"attachments\":[{\"filename\":\"lie.png\",\"mime_type\":\"image/png\",\"data_base64\":\"$JPEG_B64\"}]}"
```

**Then**：每個都回 HTTP 400 + 對應錯誤訊息（`too many attachments` / `bytes > limit` / `not in allow-list` / `magic bytes`）

---

## E2E-browser

### CU03 — Drag-drop 圖片到 textarea

**層級**：E2E-browser

**Given** 使用者開 `/chat` 頁面
**When** 從 file manager 拖一張 PNG 到 textarea 區塊
**Then**
- textarea 邊框變藍（dragOver 狀態）
- 放開後 chip 列出現 `📎 <filename> <size>`，附 ✕ 按鈕
- placeholder 文字暫變 `Drop images here…` 又恢復 `Message…`
- 送出後 chip 消失，使用者訊息顯示成 `[image:<filename>] <text>`

---

### CU04 — Paste 圖片（剪貼簿）

**層級**：E2E-browser

**Given** 使用者開 `/chat` 頁面，桌面截圖（Cmd/Ctrl+Shift+4 或 PrintScreen）放到剪貼簿
**When** 使用者點擊訊息輸入框後按 Cmd/Ctrl+V
**Then**
- chip 列出現 `📎 pasted-<timestamp>.png`（檔名依貼上時間自動命名）
- 輸入框中不出現大量亂碼或 base64 字串，保持乾淨
- 送出後依 CU01 的 placeholder 規則顯示

---

### CU05 — Discord 收圖 → ACP → 回應

**層級**：E2E-browser（含 Discord 整合）

**Given** Discord bot 上線、`DISCORD_BOT_TOKEN` 已設、bot 加進測試 channel
**When** 在 channel 傳一張 PNG（`describe this`），按 enter
**Then**
- 訊息上出現 👀
- `RunCompleted` 後 👀 → 💬 + Discord 回覆描述圖片內容
- container log 出現 `Discord ACP: attachments processed images=1 files=0 failed=0`

---

### CU06 — Discord attachment URL fetch 失敗 → text-only fallback

**層級**：E2E-browser（含 Discord 整合）

**Given** Discord bot 上線；用 `iptables` 或 `/etc/hosts` 把 `cdn.discordapp.com` 阻斷在 container 內（簡化作法：跑前先 `docker exec perch-local-test sh -c 'echo "127.0.0.1 cdn.discordapp.com" >> /etc/hosts'`，測完還原）
**When** 在 channel 傳一張 PNG + `describe`，按 enter
**Then**
- 訊息上 👀 → 💬（仍然回應）
- Discord 回覆內容是純文字 + 末尾一行 `> 附件 <filename> 下載失敗，未送進 Claude`
- log 出現 `Discord ACP: fetch attachment failed`

**Cleanup**：還原 `/etc/hosts`（`docker exec ... sed -i '/cdn.discordapp.com/d' /etc/hosts`）

---

## 備註

- AT-E01-04（acp-tool-events）涵蓋 ACP event 觸發 ManagementHub / store 寫入；本檔只驗 attachments path 的差異，不重複那些 case
- MT01-12（multi-turn-chat）純文字回歸用既有 case 驗證
- 所有 case 失敗時優先檢查：`Settings → Chat` 的 limits 是否被改過（`upload_max_files=0` 會關閉上傳）
