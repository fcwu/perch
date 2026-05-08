# Container Bootstrap 測試案例

> 功能：claude-container-bootstrap
> 涵蓋範圍：entrypoint.sh 的 cp /etc/perch-claude-host、.claude.json onboarding seed、fresh container 行為。
> 撰寫日期：2026-04-30

---

## E2E-curl — Container Bootstrap

### TBC01 — Fresh container（host ~/.claude:ro 直接 mount）能正常起動，Discord PTY 第一句有 reaction

**層級**：E2E-curl

**Given** docker-compose 掛載 `${HOME}/.claude:/etc/perch-claude-host:ro`，無 `tests/test-perchuser/.claude.json` 預先設定
**When** container 啟動後，Discord channel 送第一句訊息
**Then**
- container 啟動 log 出現 `perch listening`
- Discord 訊息在 5 秒內出現 👀 reaction（hooks 有被載入）
- `docker exec` 進容器確認 `/home/perchuser/.claude/` 為可寫目錄（非 RO mount）

**驗證指令：**
```bash
# 確認 .claude 是容器本地可寫副本，而非 RO bind mount
docker exec perch-local-test sh -c 'touch /home/perchuser/.claude/.test_write && rm /home/perchuser/.claude/.test_write && echo "writable"'
# 確認 onboarding flags 已 seed
docker exec perch-local-test cat /home/perchuser/.claude.json | python3 -m json.tool
```

---

### TBC02 — 已存在 `hasCompletedOnboarding=true` 時不被覆寫

**層級**：E2E-curl

**Given** host `~/.claude.json` 已有 `hasCompletedOnboarding=true`，entrypoint cp 後會把此值帶進容器
**When** container 啟動，entrypoint.sh 執行 jq seed
**Then** `/home/perchuser/.claude.json` 的 `hasCompletedOnboarding` 仍為 `true`（不被重寫）

**驗證指令：**
```bash
docker exec perch-local-test jq '.hasCompletedOnboarding' /home/perchuser/.claude.json
# 期望輸出：true
```

---

### TBC03 — cp 失敗時 entrypoint log warning 但仍啟動

**層級**：E2E-curl

**Given** `/etc/perch-claude-host` 不存在（未掛載 staging mount）
**When** container 啟動
**Then**
- container 正常起動，log 出現 `perch listening`
- `/home/perchuser/.claude/` 為空目錄（mkdir -p 建立）
- `/home/perchuser/.claude.json` 存在且有 onboarding flags seed

**驗證指令：**
```bash
docker logs perch-local-test 2>&1 | grep "perch entrypoint"
docker exec perch-local-test ls /home/perchuser/.claude/
docker exec perch-local-test cat /home/perchuser/.claude.json
```

---

### TBC04 — Claude Code Bash 工具能成功執行

**層級**：E2E-curl

**Given** container 使用新 mount 慣例（`/etc/perch-claude-host:ro`）啟動
**When** 透過 Chat API 送一個使用 Bash tool 的請求（例如 `echo hello`）
**Then**
- Chat API response 包含 Bash tool 的執行結果
- `session-env/` 目錄可在容器 `/home/perchuser/.claude/` 下寫入（不再 EROFS）

**驗證指令（搭配 tests/test-kb-chat-api.md 的 T55 / T56 流程）：**
```bash
# 送一個會觸發 Bash tool 的 chat request，確認 response 成功回傳
curl -s -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "run: echo hello from container"}' | jq '.response'
```

---

### TBC05 — 非 root 使用者可透過 Playwright MCP 執行 browser_navigate

**層級**：E2E-curl

**背景**：`@playwright/mcp` 在 `browser_navigate` 時會在 `PLAYWRIGHT_BROWSERS_PATH` 建立 state dir（`mcp-chrome-for-testing-*`），需要 write 權限。Chromium 在 QNAP 容器內需要 `--no-sandbox`。正確設定：`--executable-path` 指向 `/opt/ms-playwright`（read-only OK）、`PLAYWRIGHT_BROWSERS_PATH=/data/playwright`（writable volume）。

**Given** container 以 `PUID=1001` 啟動
**When** 以 uid=1001 透過 MCP protocol 呼叫 `browser_navigate`
**Then**
- MCP server 初始化成功（回傳 `serverInfo.name: "Playwright"`）
- `browser_navigate` 回傳頁面 title，無 EACCES 錯誤
- `~/.claude.json` 的 `mcpServers.playwright` 包含 `--no-sandbox`、`--executable-path` 指向 `/opt/ms-playwright` 下的 chrome binary、`env.PLAYWRIGHT_BROWSERS_PATH=/data/playwright`
- `/data/playwright` 可被 uid=1001 寫入
- Playwright plugin `.mcp.json` 與 `mcpServers.playwright` 設定一致

**驗證指令**：
```bash
CONTAINER=mykb-perch-perch-1
DOCKER="ssh home-auto source /etc/profile && /share/CACHEDEV4_DATA/.qpkg/container-station/bin/docker"

# ── 核心測試：實際呼叫 browser_navigate，確認無 EACCES ──
# 取得 MCP config args
MCP_ARGS=$(ssh home-auto "source /etc/profile && docker exec $CONTAINER \
  cat /home/perchuser/.claude.json" | \
  python3 -c "import sys,json; p=json.load(sys.stdin)['mcpServers']['playwright']; \
    print(' '.join(a for a in p['args'] if a not in ['-y','@playwright/mcp']))" )
MCP_ENV=$(ssh home-auto "source /etc/profile && docker exec $CONTAINER \
  cat /home/perchuser/.claude.json" | \
  python3 -c "import sys,json; e=json.load(sys.stdin)['mcpServers']['playwright']['env']; \
    print(' '.join(f'{k}={v}' for k,v in e.items()))")

# 用真實 MCP 設定跑 browser_navigate（這才是 agent 實際用的路徑）
cat > /tmp/test_mcp_navigate.js << 'JSEOF'
const { spawn } = require("child_process");
const args = process.argv.slice(2);
const envPairs = args.splice(0, args.indexOf("--"));
const mcpArgs = args.slice(1);
const env = { ...process.env };
envPairs.forEach(p => { const [k,v] = p.split("="); env[k] = v; });
const mcp = spawn("node", ["/usr/bin/playwright-mcp", ...mcpArgs], { env, stdio: ["pipe","pipe","pipe"] });
let out = "";
mcp.stdout.on("data", d => { out += d; });
mcp.stderr.on("data", d => process.stderr.write(d));
mcp.stdin.write(JSON.stringify({jsonrpc:"2.0",id:1,method:"initialize",params:{protocolVersion:"2024-11-05",capabilities:{},clientInfo:{name:"test",version:"1"}}}) + "\n");
mcp.stdin.write(JSON.stringify({jsonrpc:"2.0",method:"notifications/initialized",params:{}}) + "\n");
setTimeout(() => mcp.stdin.write(JSON.stringify({jsonrpc:"2.0",id:2,method:"tools/call",params:{name:"browser_navigate",arguments:{url:"https://example.com"}}}) + "\n"), 300);
setTimeout(() => {
  mcp.kill();
  const lines = out.trim().split("\n").map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(Boolean);
  const nav = lines.find(l => l.id === 2);
  if (!nav) { console.error("FAIL: no response for browser_navigate"); process.exit(1); }
  if (nav.result && nav.result.isError) { console.error("FAIL:", JSON.stringify(nav.result.content)); process.exit(1); }
  const text = nav.result?.content?.[0]?.text || "";
  if (!text.includes("Example Domain")) { console.error("FAIL: unexpected response:", text.slice(0,200)); process.exit(1); }
  console.log("PASS: browser_navigate returned page title OK");
  process.exit(0);
}, 18000);
JSEOF

scp /tmp/test_mcp_navigate.js home-auto:/tmp/test_mcp_navigate.js
ssh home-auto "source /etc/profile && docker cp /tmp/test_mcp_navigate.js $CONTAINER:/tmp/test_mcp_navigate.js && \
  docker exec -u 1001 $CONTAINER node /tmp/test_mcp_navigate.js $MCP_ENV -- $MCP_ARGS"
# 預期：PASS: browser_navigate returned page title OK

# ── 設定完整性檢查 ──
# 1. mcpServers.playwright 設定正確
ssh home-auto "source /etc/profile && docker exec $CONTAINER cat /home/perchuser/.claude.json" | \
  python3 -c "
import sys,json
p = json.load(sys.stdin)['mcpServers']['playwright']
args = p['args']
env = p['env']
assert '--no-sandbox' in args, 'missing --no-sandbox'
assert any('--executable-path' in a for a in args), 'missing --executable-path'
assert any('/opt/ms-playwright' in a for a in args), '--executable-path not pointing to /opt/ms-playwright'
assert env.get('PLAYWRIGHT_BROWSERS_PATH') == '/data/playwright', f'wrong state path: {env}'
print('PASS: mcpServers.playwright config correct')
"

# 2. /data/playwright 可被 uid=1001 寫入
ssh home-auto "source /etc/profile && docker exec -u 1001 $CONTAINER sh -c 'touch /data/playwright/.write_test && rm /data/playwright/.write_test && echo PASS: /data/playwright writable'"

# 3. plugin .mcp.json 與 mcpServers 一致（防止 agent 或 plugin 互相覆蓋）
ssh home-auto "source /etc/profile && docker exec $CONTAINER cat '/home/perchuser/.claude/plugins/marketplaces/claude-plugins-official/external_plugins/playwright/.mcp.json'" | \
  python3 -c "
import sys,json
p = json.load(sys.stdin)['playwright']
assert '--no-sandbox' in p['args'], 'plugin missing --no-sandbox'
assert any('--executable-path' in a for a in p['args']), 'plugin missing --executable-path'
assert p['env'].get('PLAYWRIGHT_BROWSERS_PATH') == '/data/playwright', f'plugin wrong state path'
print('PASS: plugin .mcp.json consistent')
"
```

**失敗時的排查順序**：
1. `browser_navigate` EACCES → 檢查 `/data/playwright` 是否為 uid=1001 writable
2. `browser_navigate` 無回應（timeout）→ 檢查 `--no-sandbox` 是否在 args、`--executable-path` 是否存在
3. Config 檢查失敗 → 確認 entrypoint.sh 的 jq patch 有執行（`docker logs` 是否有 jq warning）
