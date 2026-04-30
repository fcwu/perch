# ACP Tool Events 測試案例

> 功能：acp-tool-events / management-realtime / management-history
> 涵蓋範圍：chat-API ACP 路徑的 ACP event 驅動 ManagementHub 與 query_log_store 寫入。`tool_call_started` → `session_update`、`tool_call_completed` → tool_events row、`RunCompleted` → session_removed + status=done、`RunFailed` → status=error。
> 撰寫日期：2026-04-30
> 相關 openspec：`acp-tool-events`、`management-realtime`、`management-history`、`query-log-store`

---

## 共通前置條件

- Perch 以 `PERCH_MODE=multi` 啟動（`/ws/management` 僅在 multi mode 註冊）
- `ADMIN_TOKEN=<token>` 設定（後續以 `Authorization: Bearer <token>` 或 cookie 帶）
- `AUTH_METHOD=none` 或具備 GitLab session token（端視部署）
- chat-API 走 ACP path（chat-API 已預設 ACP-only，無 flag）

> 若部署在 single-mode 環境，AT-E01 / AT-E03 / AT-E04 中涉及 `/ws/management` 的部分視為 N/A；AT-E02 與 history-only 部分仍可跑。

---

## E2E-curl

### AT-E01 — `tool_call_started` 觸發 ManagementHub `session_update`

**層級**：E2E-curl + WS subscriber

**Given** Perch 以 multi-mode 啟動，`ADMIN_TOKEN` 設定，使用者已登入
**When**
1. 開啟 WS 訂閱 `/ws/management`（帶 admin token）
2. 觸發一個會用 Bash tool 的 chat-API query：`POST /api/chat`（body `{"query":"請執行 bash echo HELLO_AT_E01","new_conversation":true}`）

**Then** WS subscriber 收到事件序列：
1. `{"type":"session_added","session":{"id":"<uuid>","query":"請執行 bash echo HELLO_AT_E01","status":"running",...}}`
2. `{"type":"session_update","session":{"id":"<uuid>","current_tool":"Bash",...}}`（此事件來自 ACP `tool_call_started`）
3. 後續 `current_tool=""` clear 與 `session_removed`（涵蓋 AT-E03）

**驗證指令**：

```bash
# 終端 A：WS subscriber
websocat -t -E "ws://<host>/ws/management" -H "Authorization: Bearer $ADMIN_TOKEN" \
  | tee /tmp/ws-mgmt-events.jsonl

# 終端 B：觸發 query
curl -sS -X POST http://<host>/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"請執行 bash echo HELLO_AT_E01","new_conversation":true}'

# 終端 A 檢查
grep -F '"current_tool":"Bash"' /tmp/ws-mgmt-events.jsonl
# 預期：至少一筆，session_update event 帶 current_tool=Bash
```

**反向驗證**：query 不使用 tool（例如 `echo without tool` 純文字回答）→ WS 無 `session_update` event 帶非空 `current_tool`。

---

### AT-E02 — `tool_call_completed` 補完 `tool_events` row

**層級**：E2E-curl

**Given** AT-E01 已觸發，session 已結束，session_id 記為 `<sid>`
**When**
```bash
curl -sS http://<host>/api/management/history/<sid> \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '.ToolEvents'
```

**Then** 回傳 `ToolEvents` array 至少包含一筆，欄位：
- `tool_name = "Bash"`
- `started_at` non-null（來自 `tool_call_started`）
- `ended_at` non-null（來自 `tool_call_completed`，**這是本案重點**）
- `output_json` non-null/非空（包含 `HELLO_AT_E01` 字串）

**驗證**：

```bash
SID=$(curl -sS http://<host>/api/management/history?limit=1 \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.sessions[0].id')

curl -sS http://<host>/api/management/history/$SID \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq '.ToolEvents | map({tool_name, started_at, ended_at, has_output: (.output_json | length > 0)})'
# 預期：[{"tool_name":"Bash","started_at":<unix_ms>,"ended_at":<unix_ms>,"has_output":true}, ...]
```

**反向驗證**：query 完全不用工具（純對話）→ `ToolEvents` 為空 array 或 null（不出現中途斷點 row）。

---

### AT-E03 — `RunCompleted` 觸發 `session_removed` + `status=done`

**層級**：E2E-curl + WS subscriber

**Given** Perch 以 multi-mode 啟動，WS subscriber 連到 `/ws/management`
**When** 觸發任意 chat-API query（用或不用工具皆可，回應正常結束）：
```bash
curl -sS -X POST http://<host>/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"請回答：今天星期幾？","new_conversation":true}'
```

**Then**
1. WS subscriber 收到 `{"type":"session_removed","id":"<sid>","status":"done"}`
2. 緊接著 `GET /api/management/history?limit=1` 回傳的最新一筆有：
   - `status: "done"`
   - `ended_at` non-null
   - `duration_ms` > 0
   - `response` 欄位非空（內含模型回答）

**驗證**：

```bash
# 等一下讓 ACP RunCompleted flush
sleep 2

curl -sS http://<host>/api/management/history?limit=1 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq '.sessions[0] | {id, status, ended_at, duration_ms, has_response: (.response != null and .response != "")}'
# 預期：status=done, ended_at 非 null, duration_ms > 0, has_response=true
```

---

### AT-E04 — `RunFailed` / timeout 觸發 `status=error`

**層級**：E2E-curl + WS subscriber

**Given** Perch 以 multi-mode 啟動。設定 `ACP_RUN_TIMEOUT=2`（2 秒，必觸發 timeout）並重啟；或用 invalid prompt 觸發 ACP error。
**When** 觸發一個會超時的 query（例如要 Claude 用 sleep tool 跑 30 秒，但 timeout=2）：
```bash
curl -sS -X POST http://<host>/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"請執行 bash sleep 30 然後回答 done","new_conversation":true}'
```

**Then**
1. WS subscriber 收到 `{"type":"session_removed","id":"<sid>","status":"error"}`
2. `GET /api/management/history/<sid>` 回傳：
   - `status: "error"`
   - `response` 欄位含錯誤訊息（含 `timeout` 或 ACP `RunFailed` 的 error 字串）
   - `ended_at` non-null（即使 error 也要有結束時間）

**驗證**：

```bash
SID=$(curl -sS http://<host>/api/management/history?limit=1 \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.sessions[0].id')

curl -sS http://<host>/api/management/history/$SID \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq '{Status, Response, EndedAt}'
# 預期：Status=error, Response 含 "timeout" 或具體 error 字串, EndedAt 非 null
```

**Cleanup（後置）**：`PATCH /api/settings` 將 `ACP_RUN_TIMEOUT` 還原為原值（如 120 或 0 = 無 timeout），重啟。

---

## 備註：與既有測試的關係

- **T55-multi**：在 multi-mode 跑同樣的 WS subscribe + query，AT-E01 + AT-E03 已涵蓋 lifecycle 中段與末段事件
- **T56**：`/api/management/history` 列表/詳情/搜尋；AT-E02 是它的 ToolEvents 欄位細部驗證
- **MT12**：兩次 chat-API query 各自寫入 query_sessions；AT-E03 是「單次寫入」的 status=done 驗證

AT-E 系列出現 FAIL 時優先檢查：
1. ACP subprocess 是否存活（`ps aux | grep claude-agent-acp`）
2. `acp_session_pool.go` 對 chat-API key 的 acquire/release 是否正常
3. `chat_api_acp.go` 的 event handler wiring 是否註冊到 `ManagementHub` 與 `query_log_store`
