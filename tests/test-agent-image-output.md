# Agent Image Output — e2e 測試案例

> 功能：agent-image-output
> 涵蓋範圍：Agent 回應內嵌 `[image: <路徑>]` 語法 → Perch 後處理提取 token、儲存至 `<workdir>/images/<conv-id>/`、透過 `/api/images/...` 提供存取；SSE `message` 事件包含 `images` 欄位；Discord 以檔案附件傳送；圖片 store 清理。
> 撰寫日期：2026-05-08
> 相關 openspec：`agent-image-output`、`chat-ui`、`discord-acp-session`、`acp-tool-events`

---

## 共通前置條件

- Perch 以預設模式啟動（`AUTH_METHOD=none`，`PERCH_MODE=single` 或 `multi` 皆可）
- `LISTEN_ADDR=:8081`，`DB_PATH=/tmp/perch.db`，`CLAUDE_WORKDIR=<workdir>`（以下稱 `${WORKDIR}`）
- 以下測試使用 placeholder：`${HOST}=localhost:8081`、`${SESSION_COOKIE}`（`AUTH_METHOD=none` 時可省略）、`${WORKDIR}`、`${CONTAINER}`（docker 模式時）
- 測試用小圖：`tests/fixtures/tiny.png`（< 1 KB）

### 共用 helper

```bash
# 取最新一筆 SSE message 事件的 images 欄位
sse_images() {
  local conv_id="$1"
  curl -sS -N \
    -H "Cookie: session_token=${SESSION_COOKIE}" \
    "http://${HOST}/api/chat/stream?conversation_id=${conv_id}" \
    | grep '^data:' | tail -1 | sed 's/^data://' | jq '.images // []'
}

# 觸發一個 agent 查詢並等待完成（返回 conversation_id）
start_chat() {
  local query="$1" conv_id="${2:-conv-$(date +%s)}"
  curl -sS -X POST "http://${HOST}/api/chat" \
    -H "Content-Type: application/json" \
    -H "Cookie: session_token=${SESSION_COOKIE}" \
    -d "{\"query\":\"${query}\",\"conversation_id\":\"${conv_id}\",\"new_conversation\":true}" \
    | jq -r '.conversation_id // empty'
  echo "$conv_id"
}

# 列出 images 目錄內容（docker 模式）
images_dir() {
  local conv_id="$1"
  docker exec "${CONTAINER}" ls "${WORKDIR}/images/${conv_id}/" 2>/dev/null
}
```

---

## E2E-curl — 預設設定（AUTH_METHOD=none）

### AIO01 — Agent 回應含單張 `[image: /path]`，SSE message 事件有 `images` 欄位

**層級**：E2E-curl

**Given** Perch 已啟動，測試圖片 `/tmp/aio01.png` 存在於 server 端
**When** 使用者送出查詢，Agent 回應文字中包含 `[image: /tmp/aio01.png]`（可透過 mock prompt 讓 Agent 回傳此格式）
**Then**
- `/api/chat/stream` 的 SSE `message` 事件酬載包含 `"images"` 欄位，至少一個元素
- 元素格式為 `{"url": "/api/images/<conv-id>/<uuid>.png", "caption": "aio01.png"}`
- 同一 `message` 事件的 `text` 欄位**不含** `[image: /tmp/aio01.png]` 字串

**驗證指令**：

```bash
# 1. 在 server 端準備測試圖片（docker 模式）
docker exec "${CONTAINER}" cp /app/tests/fixtures/tiny.png /tmp/aio01.png

# 2. 送出查詢（需 Agent 實際回傳 [image:] 語法；可透過 system prompt 或特定指令觸發）
CONV="aio01-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請在回應中加入 [image: /tmp/aio01.png]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

# 3. 讀取 SSE stream
sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq -r 'select(.images != null and (.images | length) > 0)'
# 預期：至少一個 message 事件的 images 欄位含 url 與 caption

# 4. 確認 text 不含 [image: ...] token
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq -r '.text // ""' | grep -v '\[image:'
```

---

### AIO02 — `/api/images/<conv-id>/<filename>` 已驗證請求正常回傳圖片

**層級**：E2E-curl

**Given** AIO01 已完成，取得 `images[0].url`（格式為 `/api/images/<conv-id>/<uuid>.png`）
**When** 持有效 session cookie 的用戶端對該 URL 發 `GET` 請求
**Then**
- HTTP 200
- `Content-Type: image/png`
- `Cache-Control: private, max-age=86400`
- Response body 是合法的 PNG 位元組（首 8 bytes 為 PNG magic：`\x89PNG\r\n\x1a\n`）

**驗證指令**：

```bash
# 從 AIO01 結果取得 image URL
IMAGE_URL=$(curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq -r 'select(.images != null) | .images[0].url' | head -1)

# 帶 session cookie 存取（AUTH_METHOD=none 時可省略 -H Cookie）
curl -sS -I "http://${HOST}${IMAGE_URL}" \
  -H "Cookie: session_token=${SESSION_COOKIE}"
# 預期：HTTP/1.1 200 OK、Content-Type: image/png、Cache-Control: private, max-age=86400

# 驗證 PNG magic bytes
curl -sS "http://${HOST}${IMAGE_URL}" \
  -H "Cookie: session_token=${SESSION_COOKIE}" \
  | xxd | head -1
# 預期：00000000: 8950 4e47 0d0a 1a0a  .PNG....
```

---

### AIO03 — 未帶 session cookie 存取 `/api/images/...` → 401

**層級**：E2E-curl

**Given** AIO02 取得的 `${IMAGE_URL}` 存在，Perch 啟動時設有 session 驗證
**When** 用戶端不帶任何 Cookie 發送 `GET /api/images/<conv-id>/<uuid>.png`
**Then** Server 回應 HTTP 401 Unauthorized

**驗證指令**：

```bash
# 不帶 session cookie
curl -sS -o /dev/null -w '%{http_code}' "http://${HOST}${IMAGE_URL}"
# 預期：401

# 帶錯誤 token
curl -sS -o /dev/null -w '%{http_code}' "http://${HOST}${IMAGE_URL}" \
  -H "Cookie: session_token=invalid-fake-token"
# 預期：401
```

> **注意**：若 `AUTH_METHOD=none`，此 case 預期行為視 server 設定而定。請在 `AUTH_METHOD=gitlab` 或 `AUTH_METHOD=password` 環境下執行此測試。

---

### AIO04 — 請求不存在的圖片 → 404

**層級**：E2E-curl

**Given** Perch 已啟動
**When** 用戶端帶有效 session cookie 請求不存在的圖片路徑（conv-id 或 filename 不存在）
**Then** Server 回應 HTTP 404 Not Found

**驗證指令**：

```bash
curl -sS -o /dev/null -w '%{http_code}' \
  -H "Cookie: session_token=${SESSION_COOKIE}" \
  "http://${HOST}/api/images/nonexistent-conv/nonexistent-file.png"
# 預期：404
```

---

### AIO05 — Agent 回應含多張 `[image: ...]`，images 陣列依序包含所有附件

**層級**：E2E-curl

**Given** Perch 已啟動，server 端存在 `/tmp/aio05a.png` 與 `/tmp/aio05b.png`
**When** Agent 回應文字包含兩個 `[image: ...]` token（先 `aio05a.png` 後 `aio05b.png`）
**Then**
- SSE `message` 事件的 `images` 欄位為長度 2 的陣列
- `images[0].caption = "aio05a.png"`，`images[1].caption = "aio05b.png"`（順序依出現次序）
- 兩個 URL 均可正常存取（各自回 200）

**驗證指令**：

```bash
# 準備兩張測試圖
docker exec "${CONTAINER}" sh -c 'cp /tmp/aio01.png /tmp/aio05a.png && cp /tmp/aio01.png /tmp/aio05b.png'

CONV="aio05-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請在回應中依序加入 [image: /tmp/aio05a.png] 和 [image: /tmp/aio05b.png]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq 'select(.images != null and (.images | length) == 2) | .images | map(.caption)'
# 預期：["aio05a.png", "aio05b.png"]
```

---

### AIO06 — `[image: /tmp/nonexistent.png]` 路徑不存在 → images 欄位為空，文字正常顯示

**層級**：E2E-curl

**Given** Perch 已啟動，`/tmp/nonexistent_aio06.png` 不存在於 server 端
**When** Agent 回應包含 `[image: /tmp/nonexistent_aio06.png]`
**Then**
- SSE `message` 事件的 `images` 欄位為空陣列（`[]`）或不存在（視為空）
- `text` 欄位不含 `[image: ...]` token（token 已被刪除）
- Server log 出現 warning（路徑不存在）

**驗證指令**：

```bash
CONV="aio06-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請在回應中加入 [image: /tmp/nonexistent_aio06.png]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq '{text: .text, images: (.images // [])}'
# 預期：images 為 []，text 不含 [image: ...] 字串

# 反向：確認 text 不含 token
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq -r '.text // ""' | grep '\[image:' && echo FAIL_token_not_removed || echo OK
```

---

### AIO07 — 圖片超過 8 MB → images 欄位無該圖，text 含說明文字

**層級**：E2E-curl

**Given** Perch 已啟動，server 端存在大於 8 MB 的圖片 `/tmp/aio07_large.png`
**When** Agent 回應包含 `[image: /tmp/aio07_large.png]`
**Then**
- SSE `message` 事件的 `images` 欄位不含該圖（為空或無對應條目）
- `text` 欄位包含 `(圖片過大，無法顯示)` 取代原 token
- Server log 出現 warning（檔案過大）

**驗證指令**：

```bash
# 在 server 端建立 9 MB 假圖（不是真正的 PNG，只測大小限制）
docker exec "${CONTAINER}" dd if=/dev/zero of=/tmp/aio07_large.png bs=1M count=9

CONV="aio07-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請在回應中加入 [image: /tmp/aio07_large.png]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq '{text: .text, images: (.images // [])}'
# 預期：images 為 []，text 含 "(圖片過大，無法顯示)"

# 確認 text 含說明
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq -r '.text // ""' | grep '圖片過大' && echo OK || echo FAIL_no_message
```

---

### AIO08 — `[image: data:image/png;base64,...]` base64 內嵌格式，正常提取並可存取

**層級**：E2E-curl

**Given** Perch 已啟動
**When** Agent 回應包含 `[image: data:image/png;base64,<valid_base64_png>]`
**Then**
- Perch 解碼 base64 並儲存至 `<workdir>/images/<conv-id>/`
- SSE `message` 事件的 `images` 欄位含一個條目，`url` 格式為 `/api/images/<conv-id>/...`
- 透過該 URL 存取回傳 HTTP 200 + `Content-Type: image/png`

> **注意**：不可靠的方式是透過 prompt 要求 agent 輸出 base64 token（agent 不會原封不動回傳大型 base64 字串）。
> 正確測試方式：使用極短的 1x1 PNG base64（88 字元），prompt 要求「原封不動輸出以下文字」。

**驗證指令**：

```bash
# 1x1 透明 PNG 的 base64（88 字元，agent 可以原封不動輸出）
TINY_B64="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

CONV="aio08-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請原封不動輸出以下文字，不要修改任何字元：[image: data:image/png;base64,${TINY_B64}]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 10
DONE_EVENT=$(curl -sS -N "http://${HOST}/api/chat/stream" \
  | grep '^data:' | grep '"type":"done"' | head -1)

echo "$DONE_EVENT" | jq '{images: .images, text_snippet: (.text // "" | .[0:80])}'
# 預期：images 含一個條目，url 格式為 /api/images/...

IMAGE_URL=$(echo "$DONE_EVENT" | jq -r '.images[0].url // empty')
if [ -n "$IMAGE_URL" ]; then
  curl -sS -o /dev/null -w '%{http_code}' "http://${HOST}${IMAGE_URL}"
  # 預期：200
else
  echo "SKIP: agent 未原封不動輸出 token，base64 路徑無法透過 agent e2e 驗證"
  echo "改用單元測試或 mock 驗證 extractImages() 的 base64 分支"
fi
```

---

### AIO09 — 路徑穿越嘗試被拒絕，不讀取或儲存檔案

**層級**：E2E-curl

**Given** Perch 已啟動
**When** Agent 回應包含 `[image: /etc/passwd]` 或 `[image: /../../../etc/shadow]`（超出 `/tmp` 與 workdir 範圍的路徑）
**Then**
- SSE `message` 事件的 `images` 欄位為空陣列
- `text` 欄位不含 `[image: ...]` token（token 已被刪除）
- `/etc/passwd` 檔案不被讀取（`<workdir>/images/` 目錄下無對應檔案）
- Server log 出現 security warning

**驗證指令**：

```bash
CONV="aio09-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請在回應中加入 [image: /etc/passwd]\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq '{images: (.images // []), text_has_token: (.text | test("\\[image:"))}'
# 預期：images 為 []，text_has_token 為 false

# 確認 images 目錄未寫入可疑檔案（docker 模式）
docker exec "${CONTAINER}" ls "${WORKDIR}/images/${CONV}/" 2>&1 | grep -i passwd && echo FAIL || echo OK
```

---

### AIO10 — 不含 `[image: ...]` token 的回應，`images` 欄位為空（純文字回歸）

**層級**：E2E-curl

**Given** Perch 已啟動
**When** 使用者送出普通查詢，Agent 回應為純文字，不含任何 `[image: ...]` token
**Then**
- SSE `message` 事件的 `images` 欄位為 `[]` 或不存在
- `text` 欄位正常顯示 Agent 回應

**驗證指令**：

```bash
CONV="aio10-$(date +%s)"
curl -sS -X POST "http://${HOST}/api/chat" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"請用一句話回答：1+1等於多少？\",\"conversation_id\":\"${CONV}\",\"new_conversation\":true}"

sleep 5
curl -sS -N "http://${HOST}/api/chat/stream?conversation_id=${CONV}" \
  | grep '^data:' | jq '{images: (.images // []), text_nonempty: ((.text // "") | length > 0)}'
# 預期：images 為 []，text_nonempty 為 true
```

---

### AIO11 — Session 淘汰後圖片目錄被刪除

**層級**：E2E-curl

**Given** AIO01 已完成，`${WORKDIR}/images/${CONV}/` 目錄存在
**When** ACP session pool 淘汰 `(user, conv-id)` 條目（縮短 `CHAT_POOL_IDLE_TIMEOUT` 或手動觸發 eviction）
**Then**
- `${WORKDIR}/images/${CONV}/` 目錄不再存在
- Server log 出現 `cleaned images dir on evict`（或類似訊息）

**驗證指令**：

```bash
# 確認 images 目錄存在（前置）
docker exec "${CONTAINER}" ls "${WORKDIR}/images/${CONV}/" | grep '.png' || { echo FAIL pre-evict; exit 1; }

# 等待 idle timeout（調短 CHAT_POOL_IDLE_TIMEOUT=60 並 sleep 70）
sleep 70

# 確認目錄已刪除
docker exec "${CONTAINER}" ls "${WORKDIR}/images/${CONV}/" 2>&1 | grep -q "No such file" && echo OK_REMOVED || echo FAIL_still_present

# 確認 log
docker logs "${CONTAINER}" --tail 50 | grep -i "cleaned images dir\|images.*evict"
```

---

### AIO12 — Perch 啟動時孤兒 images 目錄超過 TTL 被刪除

**層級**：E2E-curl

**Given** `${WORKDIR}/images/` 下存在一個 mtime 超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS` 的舊目錄（以 `touch -d "30 days ago"` 模擬）
**When** Perch 重新啟動
**Then**
- 過期的孤兒目錄被刪除
- 近期目錄不受影響
- Container log 出現孤兒清理相關訊息

**驗證指令**：

```bash
# 建立孤兒目錄（backdated）
docker exec "${CONTAINER}" sh -c "
  mkdir -p ${WORKDIR}/images/aio12-orphan && \
  echo old > ${WORKDIR}/images/aio12-orphan/x.png && \
  touch -d '30 days ago' ${WORKDIR}/images/aio12-orphan ${WORKDIR}/images/aio12-orphan/x.png && \
  mkdir -p ${WORKDIR}/images/aio12-fresh && \
  cp /tmp/aio01.png ${WORKDIR}/images/aio12-fresh/recent.png
"

# 重啟 perch
docker restart "${CONTAINER}"
sleep 5

# 孤兒目錄應已消失，fresh 目錄保留
docker exec "${CONTAINER}" ls "${WORKDIR}/images/" | grep aio12-orphan && echo FAIL_orphan_kept || echo OK_orphan_removed
docker exec "${CONTAINER}" ls "${WORKDIR}/images/" | grep aio12-fresh || echo FAIL_fresh_removed

# Log 確認
docker logs "${CONTAINER}" --tail 50 | grep -i "orphan\|cleanup"
```

---

## E2E-browser — 預設設定（AUTH_METHOD=none）

### AIO13 — Web chat 訊息氣泡在文字下方渲染圖片

**層級**：E2E-browser

**Given** 使用者已開啟 `http://${HOST}/chat`，AIO01 已完成（conv-id 已知）
**When** 使用者切換到含圖片回應的對話，等待頁面完整載入
**Then**
- 訊息氣泡顯示 markdown 渲染後的文字
- 文字下方顯示圖片，圖片正常顯示（無破圖 icon）
- 圖片下方或旁邊可見原始檔名作為說明文字（例如 `aio01.png`）

---

### AIO14 — 多張圖片依序垂直排列

**層級**：E2E-browser

**Given** AIO05 已完成（conv-id 含兩張圖片的回應）
**When** 使用者切換到該對話，等待頁面完整載入
**Then**
- 訊息氣泡文字下方依序顯示兩張圖片
- 第一張圖片對應 `aio05a.png`，第二張對應 `aio05b.png`（依出現順序排列）
- 兩圖垂直堆疊排列（不並排）

---

### AIO15 — 圖片 URL 失效（圖片被刪除後）顯示破圖佔位符

**層級**：E2E-browser

**Given** AIO01 的圖片已在 server 端被手動刪除（從 server 端移除 `${WORKDIR}/images/${CONV}/` 目錄），使用者重新整理頁面後仍瀏覽含該對話的頁面
**When** 使用者瀏覽含該圖片的對話
**Then**
- 圖片位置顯示瀏覽器預設破圖佔位符（broken image icon）
- 圖片原本的 caption 文字仍可見（作為備用說明）
- 頁面其餘文字內容正常顯示，不出現錯誤頁面

---

### AIO16 — 無圖片回應時訊息氣泡僅顯示文字，無多餘圖片區塊

**層級**：E2E-browser

**Given** AIO10 的對話（純文字回應，無 `[image: ...]` token）
**When** 使用者瀏覽該對話
**Then**
- 訊息氣泡僅顯示 markdown 文字
- 氣泡區域中沒有任何圖片顯示（與有圖片時行為明確區分）

---

### AIO19 — 重新整理頁面後，歷史訊息的圖片仍正常顯示

**層級**：E2E-browser

**Given** 使用者已在 `/chat?id=<uuid>` 頁面，且該對話的助理回覆中包含圖片（圖片在文字下方可見）
**When** 使用者按下 F5 或 Ctrl+Shift+R 執行強制重新整理
**Then**
- 頁面重載完成後，助理回覆的文字內容仍正常顯示
- 助理回覆中的圖片在文字下方仍正常顯示（無破圖 icon）
- 使用者不需要重新送出訊息，圖片即出現

---

### AIO20 — 直接以帶 id 的 URL 開啟含圖片的對話，歷史圖片正常顯示

**層級**：E2E-browser

**Given** 已存在一個助理回覆含圖片的對話，其 id 為 `<uuid>`
**When** 使用者在新分頁或新視窗直接輸入 `/chat?id=<uuid>` 並開啟
**Then**
- 頁面載入後，歷史訊息（含使用者查詢與助理回覆）顯示在 chat area
- 助理回覆中的圖片在文字下方正常顯示（無破圖 icon）
- 使用者不需要做任何額外操作，圖片即出現

---

## E2E-browser — Discord 整合（需 Discord bot 上線）

### AIO17 — Discord session：Agent 輸出圖片以檔案附件方式傳送

**層級**：E2E-browser（含 Discord 整合）

**Given** Discord bot 上線，`DISCORD_BOT_TOKEN` 已設，bot 加進測試 channel（`#myprivate2`，channel ID `1496644257149353994`）
**When** 在 channel 送出指令讓 Agent 回應含 `[image: /tmp/aio01.png]`，並等待 Agent 回覆
**Then**
- Discord 回覆訊息同時包含文字與圖片附件
- 圖片附件的檔名對應原始 caption（例如 `screenshot.png`）
- Container log 出現 `Discord ACP: image attachments sent count=1`（或類似訊息）

**驗證指令**：

```bash
# 確認 bot 最新訊息包含附件
CHANNEL_ID="1496644257149353994"
curl -sS -H "Authorization: Bot ${DISCORD_BOT_TOKEN}" \
  "https://discord.com/api/v10/channels/${CHANNEL_ID}/messages?limit=5" \
  | jq '.[0] | {content: .content, attachments: [.attachments[]?.filename]}'
# 預期：attachments 含圖片檔名

docker logs "${CONTAINER}" --tail 50 | grep -i "discord.*image\|image.*discord\|attachments.*count"
```

---

### AIO18 — Discord session：圖片超過 8 MB 略過，文字末尾附說明

**層級**：E2E-browser（含 Discord 整合）

**Given** Discord bot 上線，server 端存在 `/tmp/aio07_large.png`（>8 MB）
**When** Agent 回應包含 `[image: /tmp/aio07_large.png]`
**Then**
- Discord 回覆訊息**只有文字**，無圖片附件
- 文字末尾附加 `(圖片過大，無法傳送至 Discord)`
- Container log 出現 warning（圖片過大）

**驗證指令**：

```bash
CHANNEL_ID="1496644257149353994"
curl -sS -H "Authorization: Bot ${DISCORD_BOT_TOKEN}" \
  "https://discord.com/api/v10/channels/${CHANNEL_ID}/messages?limit=5" \
  | jq '.[0] | {content_has_note: (.content | test("圖片過大")), attachment_count: (.attachments | length)}'
# 預期：content_has_note 為 true，attachment_count 為 0
```

---

---

## E2E-browser — ToolPanel 行為

> ToolPanel（工具呼叫列）在串流期間顯示執行中工具，回應完成後應自動隱藏。
> 相關修正：`ToolPanel.tsx` line 37 條件從 `entries.length === 0` 改為 `running.length === 0`。

### AIO-TP01 — ToolPanel 在回應完成後消失

**層級**：E2E-browser

**Given** 使用者開啟 `http://${HOST}` 並傳送一個會觸發工具呼叫的查詢（例如要求執行 Bash 指令）
**When** Agent 回應完成，頁面顯示完整回應內容
**Then**
- 頁面底部的 ToolPanel（含工具名稱與 spinner 的區塊）不再顯示
- 頁面上看不到「thinking…」提示或執行中的工具名稱

**When** Agent 回應尚未完成、仍在執行工具呼叫時（串流進行中）
**Then**
- 頁面底部可見 ToolPanel，顯示目前執行中的工具名稱與 spinner

---

## 備註

- AIO01–AIO12 為 curl 層級；AIO01 是其他 curl case 的前置，需先成功。
- Discord case（AIO17–AIO18）需確認同一 bot token 只有一個 perch 進程在使用（參考 `.env.local.md` 的 bot 衝突警告）。
- `AUTH_METHOD=none` 環境下 AIO03（401 驗證）建議改用有 session 驗證的環境執行，否則標記 N/A。
- 圖片儲存路徑 `<workdir>/images/<conv-id>/` 與上傳檔案的 `<workdir>/uploads/<conv-id>/` 平行，清理機制（pool eviction + orphan scan）兩者共用 `CHAT_UPLOAD_ORPHAN_TTL_DAYS`。
- 所有 case 失敗時優先確認：
  1. Agent 回應是否實際包含 `[image: ...]` token（可透過 log 或 management history 查 raw response）
  2. `<workdir>/images/` 目錄是否有寫入權限
  3. `/api/images/` 路由是否已在 server 路由表中註冊（`grep -r "images" *_routes.go`）
