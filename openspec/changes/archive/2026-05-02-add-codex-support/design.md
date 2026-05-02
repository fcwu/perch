## Context

`runtime.go` 在 `2026-05-01-multi-agent-acp-runtime` 之後已可表達任意 ACP-compatible CLI（`ACPExecutable + ACPArgs`）。本 change 把 codex 接上來。

關鍵 pre-flight 發現（推翻舊 design.md 的 phase-2 假設）：

| 舊假設 | 實際 |
|---|---|
| `@openai/codex` 有 `codex acp` 子指令 | 沒有；只有 `codex mcp`（MCP，非 ACP）|
| 直接 `npm i -g @openai/codex` 即可 | 可，但裝完仍無 ACP 能力 |
| `codex acp` 啟動 ACP server | 不存在 |

**正確進入點**：`@zed-industries/codex-acp@0.12.0` — Zed 官方 npm wrapper：

```jsonc
// @zed-industries/codex-acp 的 package.json（pre-flight 摘要）
{
  "bin": { "codex-acp": "bin/codex-acp.js" },
  "optionalDependencies": {
    "@zed-industries/codex-acp-darwin-arm64": "0.12.0",
    "@zed-industries/codex-acp-darwin-x64":   "0.12.0",
    "@zed-industries/codex-acp-linux-arm64":  "0.12.0",
    "@zed-industries/codex-acp-linux-x64":    "0.12.0",
    "@zed-industries/codex-acp-win32-arm64":  "0.12.0",
    "@zed-industries/codex-acp-win32-x64":    "0.12.0"
  }
}
```

執行模型：`codex-acp` binary 啟動即 stdio JSON-RPC server，無 subcommand、無 `--log-level` 之類旗標需求（與 claude-agent-acp 同形）。auth 走 `OPENAI_API_KEY` env var。

### Phase 0 pre-flight（2026-05-01 已實測）

```bash
$ npm install -g @zed-industries/codex-acp @zed-industries/codex-acp-linux-x64   # platform deps 拉 native binary
$ printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{}}}' | codex-acp
{"jsonrpc":"2.0","result":{"protocolVersion":1,"agentCapabilities":{...}, "authMethods":[...], "agentInfo":{"name":"codex-acp","title":"Codex","version":"0.12.0"}},"id":1}
```

實測結果：

- **Stdio 乾淨**：JSON-RPC frames only on stdout；INFO/ERROR logs 走 stderr。**`ACPArgs: nil` 是對的**，不需 `--log-level` 之類旗標（codex-acp 唯一 CLI flag 是 `-c key=value` config override）
- **`session/new {cwd, mcpServers:[]}` ✅ 接受**（perch 既有 payload 不變）；回應含 `sessionId`、`modes`、`models`、`configOptions`
- **`promptCapabilities`**：`image: true`、`audio: false`、`embeddedContext: true`（image upload 應該支援，跟 claude/opencode 一樣）
- **`mcpCapabilities`**：`http: true, sse: false`（MCP integration future-change material）
- **`authMethods`**：
  - `chatgpt`：互動式 OAuth（`codex login`，paid ChatGPT subscription）
  - `codex-api-key`：env `CODEX_API_KEY`
  - `openai-api-key`：env `OPENAI_API_KEY` ✅ 本 change 採用
- **Authenticate flow**：`authenticate` call with `methodId: "openai-api-key"` 回 `{}` 即視為成功；perch 既有 handshake 不送 `authenticate`（claude/opencode 不需要），而 codex 也能在沒有 `authenticate` 的情況下把 `session/new` 跑完拿到 sessionId 和 modes — 但**真正打 OpenAI 的時候**（prompt 階段）才會驗 key，HTTP 401 會 surface 到 stderr。**不是 fail-fast**：bad key 不會在 handshake 階段擋下，要到 prompt 才報錯
- **Modes**：codex 的 mode 跟 claude/opencode **不同**：
  - `read-only`（預設）：只讀 workspace，編輯/網路需 approval
  - `auto`（aka Default / Agent）：可改 workspace 內檔案、跑命令；workspace 外或網路需 approval
  - `full-access`：可改任意檔案、可上網
  - perch 既有 `session/set_mode "bypassPermissions"` 對 codex 會 **error**（modeId 不存在）— 走 `acp_process.go:206` graceful warning-and-continue 既有路徑，**不影響流程啟動**，但 codex 會留在預設 `read-only` mode → **codex 預設不能編輯檔案**

### 對 perch 的影響

| 行為 | 影響 |
|---|---|
| handshake 起 session OK | ✅ 與 claude/opencode 同 |
| 對話、Q&A、純 read-only 任務 | ✅ 預設 `read-only` mode 已可用 |
| 要 codex 改檔案 / 跑 command | ✅ **可用**（codex 透過 `request_permission` 逐次 ask；perch dynamic optionId picker 自動核准） |
| Bad auth | ✅ codex-acp 在 `session/new` 階段 fail-fast `Authentication required`；perch 既有 SSE error path 直接 surface |

未來若要把 codex 提升到「first-class coding agent」（直接 auto/full-access 不再每步 ask），需要在 `acp_process.go` 把 `set_mode` 改為 runtime-aware：claude→`bypassPermissions`、opencode→（skip，沿用 default `build`）、codex→`auto` 或 `full-access`。本 change **不做**，留 follow-up — **但 dynamic optionId picker 已讓 codex 在 read-only mode 也能執行工具**，所以 first-class 提升是優化、不是阻塞。

### 過程中加進來的 in-scope ACP client fixes

QA 第一輪暴露兩個 perch ACP client 既有 bug — codex 是第一個 trigger 的 runtime（claude/opencode 用不到這兩個 path），合併進本 change：

| Bug | 問題 | 修法 |
|---|---|---|
| `acpMsg.ID *int64` 只吃 int | codex 對 agent→client 請求發 UUID 字串 ID（如 `"id":"3b51..."`），`json.Unmarshal` fail，整行 silently drop，stream 卡到 timeout | `ID` 改 `json.RawMessage`；perch 自己送的 outbound call 仍 marshal int 進去；agent→client 請求直接 echo 原始 raw ID |
| Auto-approve 寫死 `optionId: "bypassPermissions"` | claude legacy 接受；codex 只認 `approved` / `approved-execpolicy-amendment` / `abort`，回傳被 reject、tool 無法執行 | 新增 `pickAutoApproveOption(params)`：解 request 的 `options[]`，優先挑 `kind:"allow_once"` → 再挑 `"allow_always"` → 都沒才 fall back `bypassPermissions`（claude legacy 兼容） |

兩個 fix 都加 unit test（`TestPickAutoApproveOption_RuntimeShapes` 6 sub-case）；既有 ACP test 套件 + IM Discord ACP 套件 zero 影響全 PASS。

## Goals / Non-Goals

**Goals**
- 設 `AGENT_RUNTIME=codex` 後 chat-API、Discord、Telegram **真的**走 codex（OpenAI Codex via `codex-acp`）
- runtime image 預裝 `codex-acp`、不影響既有 claude / opencode 安裝
- amd64 + arm64 host 都能裝（npm `optionalDependencies` 自動挑 native）
- 對既有 claude / opencode 部署完全向下相容

**Non-Goals**
- Codex subagent / `RunAgent` 非互動模式（`codex exec`）— web `/ws` PTY 用 codex 的 case 不處理（`runtime.RunAgent` 仍可回 `codex run --agent ...` 樣板，但實際指令對應 codex 慣例與否留 future change）
- `OPENAI_API_KEY` 之外的 codex auth path（chatgpt OAuth login 透過 `codex login` 互動式登入暫不支援，headless deploy 限 API key）
- runtime per-conversation 切換
- 多 runtime 同時跑

## Architecture

### `runtime.go::loadAgentRuntime` 加 case

```go
case "codex":
    return AgentRuntime{
        Name:              "codex",
        Command:           "codex",
        ArgsEnv:           "CODEX_ARGS",
        ProjectConfigDir:  ".codex",
        ProjectConfigFile: "config.toml",
        AssetDir:          "/app/perch-codex",
        SupportsHooks:     false,
        ACPExecutable:     "codex-acp",
        ACPArgs:           nil,
    }, nil
```

> `ProjectConfigDir / File` 對應 codex CLI 慣例（`~/.codex/config.toml`）；本 change 不會主動寫入這個路徑，只保留欄位完整性。`AssetDir` 在 Dockerfile 同步 `mkdir` 一個空目錄，與 `/app/perch-claude`、`/app/perch-opencode` 並列。

### Dockerfile：npm install 加 codex-acp

```dockerfile
# before
RUN npm install -g @anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp

# after
RUN npm install -g \
    @anthropic-ai/claude-code \
    @agentclientprotocol/claude-agent-acp \
    @zed-industries/codex-acp
```

不動 sst/opencode tarball 下載步驟。npm 會根據 host arch 自動透過 `optionalDependencies` 拉對的 platform binary（`linux-x64` 或 `linux-arm64`），與 claude-agent-acp 一樣。

`/app/perch-codex/` 在 image build 時建立空目錄（與 `/app/perch-claude/`、`/app/perch-opencode/` 並列），避免 entrypoint seed 路徑出錯：

```dockerfile
COPY codex/ /app/perch-codex/
```

> 若 repo 還沒 `codex/` 目錄，先建一個 placeholder（`.gitkeep` 或 minimal `README.md` 解釋 codex runtime 預載點），確保 `COPY` 不 fail。

### Auth flow

```bash
docker run -d \
  -e AGENT_RUNTIME=codex \
  -e OPENAI_API_KEY=sk-... \
  perch:latest
```

`codex-acp` 直接讀 `OPENAI_API_KEY`；perch 不需中介，`acp_process.go::buildEnv` 已 inherit container env，env var 會自動傳進 subprocess。

> 若 phase 0 實測 `codex-acp` 也接受 `OPENAI_BASE_URL`（自架 proxy 或 Azure OpenAI），順便文件化；本 change 不主動 wire。

### Settings UI

`SettingsPanel.tsx` 既有 `agent.runtime` RadioGroup 從 `[claude, opencode]` 變成 `[claude, opencode, codex]`。沒有額外的 description / 警語要動（既有 `restart-required` 機制照舊）。

### tests

- **Unit**: `runtime_test.go::TestLoadAgentRuntime_ACPFields` 加 codex sub-case
- **e2e**: `tests/test-codex-runtime.md`
  - CX01：`AGENT_RUNTIME=codex` baseline，prompt → 收到 codex completion（簡單 echo prompt 即可）
  - CX02：chat-API 起 ACP session 確認 spawn `codex-acp`（檢查 `docker exec` 拿 `ps aux` 或 startup log 行）
  - CX03：`OPENAI_API_KEY` 未設時 codex-acp 應該 fail-fast，perch 應 surface error 到 chat UI（不是 silent hang）
  - CX04：`ACP_EXECUTABLE=codex-acp ACP_EXECUTABLE_ARGS=["--debug"]` 仍生效（D6 precedence regression）
  - 對比 round：MR01 / MT12 / T55-multi sanity 確認 claude / opencode 路徑無 regression

## Decisions

### D1：用 `@zed-industries/codex-acp` 而非 community `cola-io/codex-acp`

理由：
- Zed 是 ACP 標準的提案者 + co-maintainer，wrapper 可信度最高
- npm 安裝、`optionalDependencies` 自動挑 platform binary — 與 perch 既有 `claude-agent-acp` install pattern 完全一致，Dockerfile 改動最小
- `cola-io/codex-acp` 需 Rust toolchain build from source，會把 Dockerfile builder stage 變複雜（額外 +200MB build deps），且該專案無 npm 發佈

### D2：codex-acp 不需 `--log-level` 等旗標

理由：
- pre-flight npm metadata 看到 `bin/codex-acp.js` 只是 platform shim，沒有額外 startup args 慣例
- claude-agent-acp 也是直接執行（無 args）— pattern 對齊
- 若 phase 0 實測發現 stdout log 污染（類似 opencode 的 INFO log 問題），再補 `ACPArgs`（但現有 npm 規格沒看到對應旗標 — 預期 zed-industries 已內建乾淨的 stdio handling）

### D3：保留 `Command: "codex"` + `ArgsEnv: "CODEX_ARGS"` 但本 change 不接 web `/ws` PTY

理由：
- 為 future change 留口子（`RunAgent` 變 codex subagent mode）
- web `/ws` PTY 路徑（`pty_session.go` 透過 `r.Command + r.MainArgs()`）若使用者誤切 `AGENT_RUNTIME=codex` 會 spawn 互動式 codex CLI — 行為「能跑、但跟 claude/opencode 體驗不同」可接受。Settings UI 該欄位 description 不特別警告
- 若 phase 0 發現 codex 互動式 CLI 體驗對 web `/ws` 嚴重崩壞（例如不支援 stdin pipe），補一行警語到 Settings UI；不阻塞本 change

### D4：`OPENAI_API_KEY` 是唯一 supported auth path

理由：
- ChatGPT OAuth（`codex login` 互動式）需在 host 跑一次再掛 `~/.codex/auth.json` 進容器 — 與 opencode 的「mount `~/.local/share/opencode/auth.json`」同 pattern，但對 perch headless 場景不友善
- 留給 future change：若有需求再做（類比 opencode 既有的 README 說明）

### D5：env override precedence 不變

維持 `2026-05-01-multi-agent-acp-runtime` D6 設計：

```
runtime.ACPExecutable + runtime.ACPArgs (default per AGENT_RUNTIME)
    ↑ override by:
ACP_EXECUTABLE (env, single string)
    ↑ override by:
ACP_EXECUTABLE_ARGS (env, JSON array)
```

codex 對應 default 是 `codex-acp`（無 args）；override 仍可指 fork 路徑或外部 mock。

### D6：image 體積接受 +30~50MB

`@zed-industries/codex-acp@0.12.0` 加上對應 `linux-x64` 或 `linux-arm64` native binary，估計 +30~50MB。三個 runtime 共存 image 約 +180MB（vs base ubuntu+go）。可接受、不需 image slim。

> 若未來再加 `gemini-cli` / `qwen-code` 等持續 stack，再評估「runtime per-image」compile-time pick。

## Risks / Trade-offs

- **codex-acp 的 wire 行為與 perch 預期不一致**：例如 `session/new` payload 多/少欄位、tool call event 命名、prompt_capabilities 沒 image 等；phase 0 實測類比 opencode 那次 pre-flight 流程，FAIL 的部份開 follow-up（不阻塞 merge）
- **OPENAI_API_KEY rate limit**：codex 走 OpenAI API → 如果 ratelimit 撞到 codex-acp 應回 ACP error，perch 要 propagate 到 chat UI；既有 ACP error path 應該足夠（與 claude/opencode 同），phase 0 觀察
- **codex tool catalogue 與 claude 不同**：例如 codex 預設可能不能改檔案、或 sandbox policy 與 claude 不同 — 影響 perch 的「coding agent」體驗。本 change 不調整任何 sandbox 設定，使用者自己拿捏
- **`@zed-industries/codex-acp` 仍 0.x**：API 可能斷掉。perch package-lock 不釘版本（`npm install -g` 拉 latest）— 若需穩定可改 pin（例如 `@zed-industries/codex-acp@^0.12.0`），但統一風格 perch 都拉 latest，不破例
- **codex-acp 與本機 codex CLI 是否衝突**：image 同時裝 `@openai/codex`（互動式 CLI）+ `@zed-industries/codex-acp`（ACP wrapper）會不會有 binary 名衝突？pre-flight 看 npm metadata `bin: codex-acp` — 與 official `codex` 不衝突。本 change 暫不裝 official `@openai/codex`，因為 web `/ws` PTY 用 codex 的場景留 future change

## Migration Plan

無 schema migration。Image rebuild 後：

1. 既有部署 `AGENT_RUNTIME` 未設或 `claude` / `opencode` → 行為完全不變
2. 新部署 `AGENT_RUNTIME=codex` + `OPENAI_API_KEY` → chat-API / Discord / Telegram 走 codex
3. Settings UI 的 RadioGroup 多一個 `codex` 選項；切完按 Save & Restart 走既有流程

Release note 標：

> **Feature**：新增 `AGENT_RUNTIME=codex`（透過 `@zed-industries/codex-acp`）。需設 `OPENAI_API_KEY`。Settings UI Agent Runtime 多一個選項；切換需重啟。既有 claude / opencode 部署不受影響。

## Open Questions

- ~~**Q1：codex-acp 是否預設把 INFO log 寫 stdout 污染 ACP 流？**~~ **已決（pre-flight 2026-05-01）**：不會。stdout 純 JSON-RPC，logs 走 stderr。`ACPArgs: nil` 對。
- ~~**Q2：codex-acp 對 `session/set_mode` 的回應？**~~ **已決（pre-flight 2026-05-01）**：codex 模式是 `read-only`/`auto`/`full-access`，與 claude 的 `bypassPermissions` 不同。perch 既有 set_mode 會 error，走 graceful warning-and-continue 既有路徑（不需改 code）。**副作用**：codex 預設停在 `read-only`，無法編輯檔案。本 change 接受此限制；first-class 編輯能力留 future change（runtime-aware modeId mapping）。
- **Q3：要不要同時裝 `@openai/codex` 互動式 CLI？** 暫不裝。本 change 只覆蓋 ACP path；web `/ws` PTY 用 codex 互動式 CLI 留 future change（屆時 Dockerfile 加 `@openai/codex` 並設 `Command: "codex"` 對應的 PTY 參數）
- **Q4：codex auth via ChatGPT OAuth（mount `~/.codex/auth.json`）要不要本 change 順便文件化？** 暫不。先確保 API key 路徑跑得通；OAuth path 留 README follow-up
- **Q5：是否該在 README「Agent Runtime」段補 codex 與 claude / opencode 的「能力差異表」（image support / tool support / sandbox 預設）？** **已決**：寫一個簡表，明示 codex 預設 read-only。
- **Q6：bad `OPENAI_API_KEY` 不 fail-fast 怎麼處理？** 不在本 change 處理。perch 既有 prompt-error path 已 surface error 到 chat UI（401 會變成 ACP error response，propagate 到 frontend）。CX03 改測「prompt 時 surface error」而非「handshake fail-fast」。
