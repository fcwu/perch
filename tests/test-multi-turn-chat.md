# Multi-Turn Chat 測試案例

> 功能：multi-turn-chat
> 規格來源：`openspec/changes/multi-turn-chat/specs/multi-turn-chat/spec.md`
> 撰寫日期：2026-04-19

---

## 測試層級說明

| 層級 | 說明 |
|------|------|
| **Unit** | Go unit test，直接測試 `GetRecentHistory`、`buildPrompt` 等函式，注入 fake DB |
| **Integration** | 啟動 in-process HTTP server（`httptest`），使用真實 SQLite（in-memory），curl/Go HTTP client 呼叫 |
| **E2E-curl** | 啟動真實 binary，curl 驗證 HTTP 行為，DB 需預先填入測試資料 |
| **E2E-browser** | 瀏覽器手動操作，驗證前端渲染行為 |

---

## MT02 — 24 小時以外的 session 不注入

**層級**：Unit（直接測 `GetRecentHistory` 的 WHERE 條件）或 Integration（in-memory SQLite 插入舊資料）

**目的**：確認超過 24 小時的舊 session 不會被列入歷史。

**前置條件**：
- 資料庫中有一筆 `started_at < now - 24h` 的 done session
- 該使用者無任何 24 小時內的 done session

**步驟**：
```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"query": "你記得我上次說了什麼？"}'
```

**預期**：Agent 回應不含任何歷史前置內容（從空白上下文開始）。

---

## MT03 — 無歷史記錄時不加前置

**層級**：Unit 或 Integration

**目的**：確認全新使用者（無任何 done session）的查詢不會收到歷史前置。

**前置條件**：使用者在 `query_sessions` 中無任何記錄。

**步驟**：
```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"query": "你好，我是新使用者"}'
```

**預期**：Agent 正常回應，不帶任何 `<conversation_history>` 區塊。

---

## MT04 — 歷史最多 20 筆

**層級**：Unit（直接測 `GetRecentHistory` 的 LIMIT 參數）

**目的**：確認超過 20 筆 done session 時，只取最近 20 筆。

**前置條件**：資料庫中有該使用者 24 小時內的 25 筆 done session。

**驗證方式**：
- Unit：在 fake DB 插入 25 筆，呼叫 `GetRecentHistory`，斷言回傳長度 = 20 且為最新的 20 筆。
- Integration：in-memory SQLite 插入 25 筆，確認 `buildPrompt` 產生的 `<conversation_history>` 只含 20 對。

**預期**：最舊的 5 筆被排除，只有最近 20 筆被注入。

---

## MT05 — new_conversation=true 略過歷史查詢

**層級**：Integration（可 spy `GetRecentHistory` 呼叫次數）

**目的**：確認帶 `new_conversation: true` 的請求不會注入任何歷史。

**前置條件**：使用者有近期 done session。

**步驟**：
```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"query": "你好", "new_conversation": true}'
```

**預期**：
- `GetRecentHistory` 未被呼叫（可透過 mock/spy 或日誌驗證）
- Agent 收到的 prompt 不含 `<conversation_history>` 區塊

---

## MT06 — 省略 new_conversation 欄位時行為正常

**層級**：Integration

**目的**：確認 `new_conversation` 為選填，預設行為不受影響。

**步驟**：
```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"query": "hello"}'
```

**預期**：Server 正常查詢近期 session 並注入歷史（與 MT01 行為一致）。

---

## MT07 — 前端顯示多輪對話串

**層級**：E2E-browser（JS mock inject，無需真實 agent）

**目的**：確認前端以訊息串（thread）呈現多輪對話，舊訊息不被覆蓋。

**SSE 協議**（mock 依據）：
- `POST /api/chat` → `200 {"user_id":"<id>"}`
- `GET /api/chat/stream` → EventSource：
  - `event: pty\ndata: <base64(文字)>\n\n`（重複，累積助理回應）
  - `event: json\ndata: {"type":"done"}\n\n`（關閉 session）

**步驟**：
```bash
# 1. 偽造 user session cookie
COOKIE=$(go run ./cmd/mkcookie -user=99 -username=testuser -role=user -secret=$COOKIE_SECRET)

# 2. 注入 cookie 並導航至 /chat
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs cookie-set <target> perch_session "$COOKIE" <host>
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs nav <target> http://<host>/chat

# 3. 注入 JS mock，覆蓋 fetch 和 EventSource
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs eval <target> "
let callCount = 0;
const responses = ['回應 A', '回應 B'];
window._origFetch = window.fetch;
window.fetch = async (url, opts) => {
  if (url === '/api/chat') return new Response(JSON.stringify({user_id:'99'}), {status:200,headers:{'Content-Type':'application/json'}});
  return window._origFetch(url, opts);
};
window.EventSource = function(url) {
  const self = this; self.listeners = {};
  self.addEventListener = (t, fn) => { self.listeners[t] = fn; };
  self.close = () => {};
  const reply = responses[callCount++] || '回應';
  setTimeout(() => {
    self.listeners['pty']?.({data: btoa(reply)});
    setTimeout(() => self.listeners['json']?.({data: JSON.stringify({type:'done'})}), 100);
  }, 200);
  return self;
};
"

# 4. 送出訊息 A，等待回應
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs eval <target> "document.querySelector('input[placeholder]').value='訊息 A'; document.querySelector('form button, button[type=submit]').click();"
sleep 1

# 5. 送出訊息 B，等待回應
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs eval <target> "document.querySelector('input[placeholder]').value='訊息 B'; document.querySelector('form button, button[type=submit]').click();"
sleep 1

# 6. 截圖並讀取 AX tree
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs snap <target>
node ~/.agents/skills/chrome-cdp/scripts/cdp.mjs ax <target>
```

**預期**：
- AX tree 同時包含「訊息 A」、「回應 A」、「訊息 B」、「回應 B」
- 每個新回應 append 至底部，不替換先前的訊息
- 對話串可垂直捲動

---

## MT08 — 新對話按鈕清除前端並重置歷史

**層級**：E2E-browser（DevTools Network 驗證 request body）

**目的**：確認點擊「New conversation」後，前端清空且下一次查詢帶 `new_conversation: true`。

**步驟**：
1. 送出幾則訊息，讓前端有對話內容。
2. 點擊「New conversation」按鈕。
3. 送出新的查詢。

**預期**：
- 點擊後，前端訊息串立即清空。
- 第三步送出的請求 body 包含 `"new_conversation": true`（DevTools Network 驗證）。
- Server 不注入歷史，agent 從空白上下文開始。

---

## MT09 — Discord 查詢不注入對話歷史

**層級**：Unit（直接測 Discord handler 的 `StartSession` 呼叫參數）

**目的**：確認 Discord 觸發的 session 不受多輪歷史影響。

**驗證方式**：
- Unit：mock `StartSession`，斷言 Discord handler 傳入的 `newConversation=true`（或等效的無歷史路徑）。

**預期**：Discord session 的 prompt 無歷史前置。

---

## MT10 — Scheduler 查詢不注入對話歷史

**層級**：Unit（直接測 Scheduler handler 的 `StartSession` 呼叫參數）

**目的**：確認排程觸發的 session 不受多輪歷史影響。

**驗證方式**：與 MT09 相同，mock `StartSession`，斷言 scheduler 路徑不傳入歷史。

**預期**：Scheduler session 的 prompt 無歷史前置。

---

## MT11 — 歷史格式正確

**層級**：Unit（直接測 `buildPrompt` 函式的輸出字串）

**目的**：確認注入的歷史前置格式符合設計文件規範。

**驗證方式**：給定 2 筆 fake history，呼叫 `buildPrompt`，用 `strings.Contains` 或 snapshot 斷言輸出格式為：

```
<conversation_history>
User: <turn 1 query>
Assistant: <turn 1 response>
...
</conversation_history>

<current query>
```

**預期**：每個 User/Assistant 對換行分隔，整個區塊以 `<conversation_history>` / `</conversation_history>` 包裹，後接一個空行再接當前查詢。

---

## MT12 — 管理員歷史頁面不受影響

**層級**：E2E-curl 或 Integration

**目的**：確認 `/admin/history` 仍正常顯示所有 session，且儲存的 `query` 欄位不含歷史前置。

**步驟**：
1. 完成數輪對話。
2. `curl http://localhost:8080/admin/history`（或瀏覽器開啟）。

**預期**：每一輪查詢均作為獨立記錄顯示，`query` 欄位不含 `<conversation_history>` 前置（歷史注入僅在送給 agent 前發生，不影響儲存）。
