# Multi-Agent Chat — Pinning, Per-Conversation Runtime, Scheduler 測試案例

> 功能：multi-agent-chat-pinning-scheduler
> 規格來源：`openspec/changes/multi-agent-chat-pinning-scheduler/`
> 撰寫日期：2026-05-02
> 環境：`tests/.env.local.md`（直跑 binary，port 8081）

---

## 測試層級說明

| 層級 | 說明 |
|---|---|
| **Unit** | Go unit test（已涵蓋 store / scheduler / mcp 路徑），由 `go test ./...` 執行 |
| **E2E-curl** | 啟動 binary，curl 驗證 HTTP API |
| **E2E-mcp** | 直接以 stdio 跑 `./perch mcp` 子命令並送 JSON-RPC，驗證工具行為 |

---

## 啟動測試 binary

```bash
cd /home/dorowu/workspace/mykb/code/perch

# Build
go build -o perch .

# Run on port 8081 with throwaway DB
LISTEN_ADDR=:8081 \
DB_PATH=/tmp/perch-mas.db \
CLAUDE_WORKDIR=/tmp/perch-ws \
AUTH_METHOD=none \
PERCH_MODE=single \
./perch > /tmp/perch-mas.log 2>&1 &
echo $! > /tmp/perch-mas.pid

# 健康檢查
curl -s http://localhost:8081/api/auth/status | head -c 200

# 結束
kill "$(cat /tmp/perch-mas.pid)"
rm -f /tmp/perch-mas.db /tmp/perch-mas.log
```

> 備註：本測試以 `AUTH_METHOD=none` 跑單使用者模式，所有 user_id 解析為 `"default"`。多使用者隔離由 unit test 涵蓋。

---

## RT01 — `GET /api/runtimes` 回傳 registry

**層級**：E2E-curl

**Given** binary 啟動於 8081
**When** `curl -s http://localhost:8081/api/runtimes`
**Then** 回應 `{"runtimes":[{...claude...}, ...]}`，claude entry 含 `supports_mcp:true` 與兩個 `claude-*` model

```bash
curl -s http://localhost:8081/api/runtimes | jq '.runtimes[] | select(.id=="claude")'
# 預期：id=claude, supports_mcp=true, models 至少包含 claude-sonnet-4-6
```

---

## CV01 — `GET /api/conversations` 新 cursor 形狀

**層級**：E2E-curl

**Given** DB 為空
**When** `curl http://localhost:8081/api/conversations?limit=10`
**Then** 回應 JSON 含 `pinned`, `recent`, `next_before` 三個 key

```bash
curl -s http://localhost:8081/api/conversations?limit=10 | jq 'keys'
# 預期：["next_before","pinned","recent"]
```

---

## CV02 — `POST /api/chat` 新建 conversation 帶 runtime/model

**層級**：E2E-curl

**Given** DB 為空
**When** POST `/api/chat` body `{"query":"hello","runtime":"claude","model":"claude-opus-4-7"}`
**Then** 回 200，conversation 行 `runtime=claude, model=claude-opus-4-7`

```bash
RESP=$(curl -s -X POST http://localhost:8081/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"query":"hello","runtime":"claude","model":"claude-opus-4-7"}')
CONV=$(echo "$RESP" | jq -r .conversation_id)
echo "convID=$CONV"
sqlite3 /tmp/perch-mas.db "SELECT runtime, model FROM conversations WHERE id='$CONV'"
# 預期：claude|claude-opus-4-7
```

> 備註：ACP subprocess 不會真的成功 spawn（測試環境沒有 `claude-agent-acp`）；conversation 列已建立，這對驗證 schema 已足夠。

---

## CV03 — `PATCH /api/conversations/{id}` 設定 pinned

**層級**：E2E-curl

**Given** 已有 conversation $CONV
**When** PATCH `{"pinned":true}`
**Then** 回 200，row 的 `pinned=1, pinned_at` 非 null

```bash
curl -s -X PATCH "http://localhost:8081/api/conversations/$CONV" \
  -H 'Content-Type: application/json' -d '{"pinned":true}' | jq '.pinned, .pinned_at'
# 預期：true / <number>
```

---

## CV04 — `PATCH` 拒絕未知 runtime

**層級**：E2E-curl

**Given** 已有 conversation $CONV
**When** PATCH `{"runtime":"frobnicate"}`
**Then** 回 400

```bash
RESP=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "http://localhost:8081/api/conversations/$CONV" \
  -H 'Content-Type: application/json' -d '{"runtime":"frobnicate"}')
test "$RESP" = "400"  # 預期：true
```

---

## CV05 — Cursor pagination：第一頁 + Load more

**層級**：E2E-curl

**Given** insert 5 conversations 到 DB；pin 兩個
**When** `GET /api/conversations?limit=2`
**Then** 第一頁回應 `pinned` 含 2 個，`recent` 含 2 個，`next_before > 0`；用 cursor 拿第二頁

```bash
# Helper：手動插入 5 列（需要 sqlite3 CLI，預先 stop binary 也行）
for i in 1 2 3 4 5; do
  curl -s -X POST http://localhost:8081/api/chat \
    -H 'Content-Type: application/json' \
    -d "{\"query\":\"row-$i\"}" >/dev/null
  sleep 0.05  # 確保 updated_at 不同
done

# 取出兩個 conv 並 pin
ALL=$(curl -s 'http://localhost:8081/api/conversations?limit=10')
ID1=$(echo "$ALL" | jq -r '.recent[0].id')
ID2=$(echo "$ALL" | jq -r '.recent[1].id')
curl -s -X PATCH "http://localhost:8081/api/conversations/$ID1" -H 'Content-Type: application/json' -d '{"pinned":true}' >/dev/null
curl -s -X PATCH "http://localhost:8081/api/conversations/$ID2" -H 'Content-Type: application/json' -d '{"pinned":true}' >/dev/null

# 第一頁 limit=2 → 2 pinned + 2 recent + cursor
P1=$(curl -s 'http://localhost:8081/api/conversations?limit=2')
echo "$P1" | jq '{pinned: (.pinned|length), recent: (.recent|length), next_before}'
# 預期：pinned=2 recent=2 next_before>0

CURSOR=$(echo "$P1" | jq -r .next_before)
P2=$(curl -s "http://localhost:8081/api/conversations?before=$CURSOR&limit=2")
echo "$P2" | jq '{pinned: (.pinned|length), recent: (.recent|length)}'
# 預期：pinned=0（cursor pages 不重複 pinned），recent>0
```

---

## CV06 — `DELETE /api/conversations/{id}` cascade 到 schedules

**層級**：E2E-curl

**Given** conversation $CONV，並透過 POST schedules 建一筆 schedule
**When** DELETE 該 conversation
**Then** 204；該 conv 對應的 chat_schedules 全消失

```bash
# 建 schedule
SCH=$(curl -s -X POST "http://localhost:8081/api/conversations/$CONV/schedules" \
  -H 'Content-Type: application/json' \
  -d "{\"prompt\":\"daily 9am\",\"hour\":9,\"minute\":0,\"repeat\":true}")
echo "$SCH" | jq -r .id

# DELETE conv
RESP=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "http://localhost:8081/api/conversations/$CONV")
test "$RESP" = "204"

# Schedules 都消失
sqlite3 /tmp/perch-mas.db "SELECT COUNT(*) FROM chat_schedules WHERE conversation_id='$CONV'"
# 預期：0
```

---

## SCH01 — Schedule CRUD：create / list / delete

**層級**：E2E-curl

**Given** 新 conversation $CONV
**When** POST schedule (`hour:9, minute:30, repeat:true, prompt:"morning"`) → GET → DELETE → GET
**Then** create 回 201，list 含一筆，delete 回 204，再次 list 為空

```bash
NEW=$(curl -s -X POST http://localhost:8081/api/chat -H 'Content-Type: application/json' -d '{"query":"sched-host"}')
CONV=$(echo "$NEW" | jq -r .conversation_id)

# Create
SCH=$(curl -s -X POST "http://localhost:8081/api/conversations/$CONV/schedules" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"morning","hour":9,"minute":30,"repeat":true}')
SID=$(echo "$SCH" | jq -r .id)

# List
curl -s "http://localhost:8081/api/conversations/$CONV/schedules" | jq '.schedules | length'
# 預期：1

# Delete
curl -s -o /dev/null -w '%{http_code}' -X DELETE "http://localhost:8081/api/conversations/$CONV/schedules/$SID"
# 預期：204

# List again
curl -s "http://localhost:8081/api/conversations/$CONV/schedules" | jq '.schedules | length'
# 預期：0
```

---

## SCH02 — Schedule 驗證：daily + one_shot 同時存在 → 400

**層級**：E2E-curl

```bash
RESP=$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://localhost:8081/api/conversations/$CONV/schedules" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"x","hour":9,"minute":0,"one_shot_at":99999999999999}')
test "$RESP" = "400"
```

---

## SCH03 — Schedule one_shot 過去時間 → 400

**層級**：E2E-curl

```bash
RESP=$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://localhost:8081/api/conversations/$CONV/schedules" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"x","one_shot_at":1}')
test "$RESP" = "400"
```

---

## SCH04 — Cross-conversation schedule create → 404

**層級**：E2E-curl

```bash
RESP=$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://localhost:8081/api/conversations/no-such-conv/schedules" \
  -H 'Content-Type: application/json' -d '{"prompt":"x","hour":9,"minute":0}')
test "$RESP" = "404"
```

---

## MCP01 — `./perch mcp` 缺 env 立刻失敗

**層級**：E2E-mcp

```bash
PERCH_USER_ID=u1 ./perch mcp < /dev/null
# 預期：exit code != 0，stderr 含 "PERCH_CONV_ID is required"
```

---

## MCP02 — `./perch mcp` initialize + tools/list

**層級**：E2E-mcp

```bash
TMPDB=/tmp/perch-mcp.db
rm -f "$TMPDB"
# 先啟動 main binary 一次以建立 schema（也可直接 OpenStore 建立）
LISTEN_ADDR=:8083 DB_PATH="$TMPDB" AUTH_METHOD=none ./perch >/dev/null 2>&1 &
PID=$!
sleep 0.3
curl -s http://localhost:8083/api/auth/status >/dev/null
kill $PID
wait $PID 2>/dev/null

# 預先插入 conv 避免後續 schedule 觸發外鍵
sqlite3 "$TMPDB" "INSERT INTO conversations(id,user_id,title,created_at,updated_at) VALUES ('cv1','u1','t',1,1)"

cat <<'EOF' | PERCH_USER_ID=u1 PERCH_CONV_ID=cv1 PERCH_DB_PATH="$TMPDB" ./perch mcp
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
EOF
# 預期：兩行 JSON 回應；第二行 result.tools 含 schedule_message / list_schedules / cancel_schedule，
# 且任一 tool 的 inputSchema.properties 不含 user_id / conversation_id
```

---

## MCP03 — `schedule_message` 工具 INSERT 帶入 env identity

**層級**：E2E-mcp

```bash
FUTURE=$(($(date +%s%3N) + 3600000))
cat <<EOF | PERCH_USER_ID=u1 PERCH_CONV_ID=cv1 PERCH_DB_PATH="$TMPDB" ./perch mcp
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"schedule_message","arguments":{"prompt":"ping","one_shot_at":$FUTURE,"user_id":"evil","conversation_id":"otherconv"}}}
EOF

# DB 內必須是 (u1, cv1)，不被 args 偽造
sqlite3 "$TMPDB" "SELECT user_id, conversation_id FROM chat_schedules"
# 預期：u1|cv1
```

---

## ADM01 — Admin endpoints 受 auth gate

**層級**：E2E-curl

```bash
# 無 admin cookie → 401 / 403（依 adminAuth 邏輯）
RESP=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8081/api/management/conversations)
echo "RESP=$RESP"
# 預期：401 或 403（admin 未啟用時可能 200/空 — 取決於 ADMIN_TOKEN 是否設定）
```

> 備註：本地 `.env.local.md` 預設 `ADMIN_TOKEN` 不設，admin route 全開放 — 此 case 會拿到 200。為了驗 401 分支，跑 binary 時加 `ADMIN_TOKEN=secret`。

---

## ADM02 — Admin Conversations List + 單筆 + Messages

**層級**：E2E-curl（搭配 ADMIN_TOKEN 設定）

```bash
# 啟動帶 admin token
LISTEN_ADDR=:8084 ADMIN_TOKEN=secret DB_PATH=/tmp/perch-adm.db AUTH_METHOD=none ./perch >/dev/null 2>&1 &
PID=$!
sleep 0.3

# 建 1 筆 conversation 透過 chat
curl -s -X POST http://localhost:8084/api/chat -H 'Content-Type: application/json' -d '{"query":"hi"}' >/dev/null
sleep 0.3  # ACP 失敗也沒關係，conversation 已建

# 取 admin cookie
COOKIE=$(curl -s -c - -X POST http://localhost:8084/management/login -d 'token=secret' | grep perch_admin | awk '{print $7}')
echo "COOKIE=$COOKIE"

# Admin list
curl -s -H "Cookie: perch_admin=$COOKIE" 'http://localhost:8084/api/management/conversations?limit=10' | jq '.total'
# 預期：1

# Admin schedules list（空）
curl -s -H "Cookie: perch_admin=$COOKIE" 'http://localhost:8084/api/management/schedules?limit=10' | jq '.total'
# 預期：0

kill $PID
```

---

## ADM03 — Admin mutation methods 回 405

**層級**：E2E-curl

```bash
# 與 ADM02 同 binary（ADMIN_TOKEN=secret）
COOKIE=$(curl -s -i -X POST -H 'Content-Type: application/json' -d '{"token":"secret"}' http://localhost:8084/management/login \
  | grep -i 'set-cookie' | sed -n 's/.*perch_admin=\([^;]*\).*/\1/p' | head -1)

PATCH=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH -H "Cookie: perch_admin=$COOKIE" -H 'Content-Type: application/json' -d '{}' 'http://localhost:8084/api/management/conversations/foo')
DEL=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "Cookie: perch_admin=$COOKIE" 'http://localhost:8084/api/management/schedules/foo')
POST=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Cookie: perch_admin=$COOKIE" 'http://localhost:8084/api/management/schedules')
echo "PATCH=$PATCH DELETE=$DEL POST=$POST"
# 預期：405 405 405
```

---

## UI01 — `/management` 顯示 Conversations / Schedules 兩個 tab

**層級**：手動／chrome-cdp

啟動 binary 與 ADM02 相同（`ADMIN_TOKEN=secret`），瀏覽器登入 `/management/login`，輸入 `secret`，重定向到 `/management`，預期看到上方 tab：

```
[Live] [Conversations] [Schedules] [History] [Analytics]
```

點 Conversations → 顯示 `id, user, title, runtime, model, updated`。點任一筆 → 顯示 messages（含 `source='schedule'` 的 ⏰ badge 若有）。
點 Schedules → 顯示 `user, conversation_id, trigger, prompt, enabled, last_fired_at`。**不應**出現任何 edit / delete / pause 按鈕。

---

## 結論判定

每個 case 在 `Then` 行都標明預期值；測試腳本應在每步驗證 exit code == 0 後再進行下一步。失敗時印出實際輸出。整套以單一 shell session 跑完並產生 PASS/FAIL summary。
