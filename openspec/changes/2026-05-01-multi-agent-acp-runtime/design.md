## Context

`runtime.go` 已有 runtime abstraction，但 ACP path 完全繞過它：

```go
// runtime.go
type AgentRuntime struct {
    Name              string   // "claude" | "opencode"
    Command           string   // "claude" | "opencode"
    ArgsEnv           string   // "CLAUDE_ARGS" | "OPENCODE_ARGS"
    DefaultEnv        []string // claude-specific env hints
    ProjectConfigDir  string   // ".claude" | ".opencode"
    ProjectConfigFile string   // "settings.json" | ".opencode.json"
    AssetDir          string   // "/app/perch-claude" | "/app/perch-opencode"
    SupportsHooks     bool     // true | false  (legacy hook era; now unused)
}
```

```go
// acp_process.go:78-83
executable = os.Getenv("ACP_EXECUTABLE")
if executable == "" {
    executable = "claude-agent-acp"
}
```

```go
// im_discord.go:262
acpExecutable string // path to claude-agent-acp binary (default from ACP_EXECUTABLE / "claude-agent-acp")
```

ACP subprocess executable 與 `AgentRuntime` **完全脫節**。若要把它們連起來，`AgentRuntime` 需要新增足以驅動 ACP 路徑的欄位。

ACP CLI 慣例（不同 agent 略有差異，需驗證後才寫死）：

| Agent | binary | ACP server 啟動方式 |
|-------|--------|------|
| claude-agent-acp | `claude-agent-acp` | 直接執行（npm package `@agentclientprotocol/claude-agent-acp`）|
| sst/opencode | `opencode` | `opencode acp` subcommand |
| codex (Phase 2) | `codex` | `codex acp`（待 Phase 2 verify）|

## Goals / Non-Goals

**Goals**
- 設 `AGENT_RUNTIME=opencode` 後 chat-API、Discord、Telegram **真的**走 OpenCode（而非 Claude）
- Dockerfile 在 amd64 / arm64 host 都能裝出可執行的 opencode binary
- runtime 抽象擴充欄位後足以容納未來新 agent（codex 等）— 設計留口子，但不在本 change 實作
- 對既有 claude-only 部署完全向下相容（`AGENT_RUNTIME` 未設或為 `claude` 時行為不變）

**Non-Goals**
- Per-conversation runtime 切換（一個 perch process 只認一個 runtime）
- 不同 runtime **同時**跑（multiplex）
- 非 ACP-compatible agent（Aider、Continue 等）
- runtime 的 `permissionMode` / `system_prompt` / `workspace_path` 等 `new_session` 參數抽象（claude vs opencode 對 `new_session` payload 接受度可能不同；本 change 假設 claude 的參數對 opencode 也夠用，phase 0 verify）

## Architecture

### AgentRuntime 擴充

```go
type AgentRuntime struct {
    // existing fields …

    // ACP-specific
    ACPExecutable string   // e.g. "claude-agent-acp", "opencode"
    ACPArgs       []string // extra args before ACP starts speaking JSON-RPC, e.g. []string{"acp"} for opencode
}
```

`loadAgentRuntime`：

```go
case "claude":
    return AgentRuntime{
        // ...
        ACPExecutable: "claude-agent-acp",
        ACPArgs:       nil,
    }, nil
case "opencode":
    return AgentRuntime{
        // ...
        ACPExecutable: "opencode",
        ACPArgs:       []string{"acp"},
    }, nil
```

### ACP path wiring

`acp_process.go::NewACPProcess` 簽名：

```go
// before
func NewACPProcess(workdir string, executable string, logger *slog.Logger) *ACPProcess

// after
func NewACPProcess(workdir string, executable string, args []string, logger *slog.Logger) *ACPProcess
```

`exec.Command(executable, args...)` 啟動 subprocess（`args` 在進入 stdio JSON-RPC 之前已被 binary 消耗，例如 `opencode acp` 進入 ACP mode 後 stdin/stdout 自動是 JSON-RPC）。

`acp_session_pool.go::newACPSessionPool` 已經有 `executable string` 參數；改成 `executable string, extraArgs []string` 並 forward 進 process。

### 起 pool 的時機

```go
// chat_api_acp.go::newACPUserSessionManager (現況讀 hard-coded "")
pool := newACPSessionPool("", workdir, logger)

// 改成
pool := newACPSessionPool(runtime.ACPExecutable, runtime.ACPArgs, workdir, logger)
```

`im_discord.go::DiscordSessionManager` 同步：把 `acpExecutable string` 換成 `runtime AgentRuntime`（已存在），起 session 時把 ACP 兩個欄位傳給 pool。

`ACP_EXECUTABLE` env var 保留作 dev override：

```go
// acp_process.go - inside NewACPProcess:
if v := os.Getenv("ACP_EXECUTABLE"); v != "" {
    executable = v // dev override
}
```

### Dockerfile：multi-arch + 換 fork

```dockerfile
# before
RUN curl -fsSL https://api.github.com/repos/anomalyco/opencode/releases/latest | \
    jq -r '(.assets[] | select(.name=="opencode-linux-arm64.tar.gz") | .browser_download_url)' | \
    xargs -I {} ... tar -xzf ...

# after
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in \
      amd64) OC_ASSET="opencode-linux-x64.tar.gz" ;; \
      arm64) OC_ASSET="opencode-linux-arm64.tar.gz" ;; \
      *) echo "unsupported arch $ARCH for opencode" && exit 1 ;; \
    esac && \
    curl -fsSL https://api.github.com/repos/sst/opencode/releases/latest | \
    jq -r --arg n "$OC_ASSET" '(.assets[] | select(.name==$n) | .browser_download_url)' | \
    xargs -I {} sh -lc 'tmp=$(mktemp -d) && curl -fsSL "{}" -o "$tmp/opencode.tgz" && tar -xzf "$tmp/opencode.tgz" -C /usr/local/bin && chmod +x /usr/local/bin/opencode && rm -rf "$tmp"'
```

claude-agent-acp 仍透過 npm 裝，與 opencode 共存；image 同時帶兩個 runtime 的 binary，由 `AGENT_RUNTIME` 在 startup 決定誰活。

## Decisions

### D1：ACP executable 的決定權從 env 移到 `AgentRuntime`

理由：
- `AGENT_RUNTIME` 是使用者意圖、`ACP_EXECUTABLE` 是實作細節 — 後者繞過前者是 design smell
- 保留 `ACP_EXECUTABLE` 做 override（dev 自製 fork、或 CI 跑 mock subprocess 時用），但語意明確標 override

### D2：claude-agent-acp 與 opencode binary 同存於 image

理由：
- 切 runtime 不需重 build image
- 兩個 binary 加總 ~150MB（claude-agent-acp via npm 約 50MB、opencode tarball 約 100MB），可接受
- 未來加 codex 也是同一 pattern（一直 stack 下去到底再評估 image slim）

### D3：本 change 不抽象 `new_session` 參數

`new_session` 目前傳 `permissionMode: "bypassPermissions"` + `workspace_path`。OpenCode 的 ACP server 是否接受這兩個欄位需 phase 0 實測；如果不接受，issue 一個 follow-up change 抽象 `RuntimeSessionParams`。本 change 假設「兩個 agent 都接受」。

### D4：Dockerfile arch detect 用 `dpkg --print-architecture`

`dpkg` 在 ubuntu base image 是預裝。比 `uname -m` 直觀（後者回 `x86_64` 不是 `amd64`，要再 map）。`alpine` 沒 dpkg 但 perch image 是 ubuntu，OK。

### D5：Settings UI 不警告「切 runtime 需重啟」UI flow

`agent.runtime` 早就標 `restart-required`（`settings.go:25` 註釋）；既有 Save & Restart 流程已涵蓋。本 change 只刪掉誤導性的「OpenCode 限 web /ws」說法，不新增 UI flow。

### D6：env override 的 precedence

```
runtime.ACPExecutable + runtime.ACPArgs (default per AGENT_RUNTIME)
    ↑ override by:
ACP_EXECUTABLE (env, single string — sets executable, args 不變)
    ↑ override by:
ACP_EXECUTABLE_ARGS (env, JSON array — 可同時覆蓋 args；保留作高階 dev 用)
```

`ACP_EXECUTABLE_ARGS` 為新環境變數，文件可寫成低調 troubleshooting 選項。

## Risks / Trade-offs

- **OpenCode ACP 與 claude-agent-acp 的 wire 差異**：兩者都實作 ACP，但 capability flags、tool name 命名、`session/update` 細節可能略有出入（例如 image content 的 `_meta.claudeCode.toolName` 是 claude 專有）。Phase 0 跑 perch 既有 ACP test suite（CU01-06、AT-E01-04）對 opencode 確認哪些 PASS / FAIL；FAIL 的部份開 follow-up tickets，不阻塞本 change merge（本 change 只承諾「能跑 prompt」）
- **Image 體積**：兩個 runtime 共存約 +100MB；未來 +codex 再 +30~50MB。可接受但要監控
- **OpenCode 路徑沒有 `_meta.claudeCode.toolName`**：tool_call event 的 toolName 取得方式可能不同；ManagementHub `current_tool` 顯示對 opencode session 可能會 empty。Phase 0 量
- **Dockerfile arch detect 失敗**：build 在非 amd64/arm64 平台會直接 fail（明示的 `exit 1`）。對 QNAP（arm64）、AWS Graviton（arm64）、x86 dev box（amd64）都覆蓋，符合 perch 既有部署平台
- **`anomalyco/opencode` 與 `sst/opencode` 不是同一份 binary**：URL/CLI 介面/版本可能完全不同。換掉 fork 後執行行為可能變化。Phase 0 在 :8081/:8082 測 web `/ws` 一次 `opencode --help` 確認

## Migration Plan

無 schema migration。Image rebuild 後：

1. 既有部署 `AGENT_RUNTIME` 未設或為 `claude` → 行為不變，不感知本 change
2. 既有部署 `AGENT_RUNTIME=opencode` → web `/ws` PTY 從 broken arm64 binary 變可用 amd64/arm64；chat-API / IM 從「靜默用 claude」變「真的用 opencode」（**行為改變但合理**）
3. 沒設 `AGENT_RUNTIME` 但設了 `ACP_EXECUTABLE` 的 dev override → 仍然 override 生效

Release note 標：

> **Behavioural change**：若 `AGENT_RUNTIME=opencode` 且 `ACP_EXECUTABLE` 未設，chat-API / Discord / Telegram 從本版起會 spawn `opencode acp` 而非 `claude-agent-acp`。要保持舊「靜默用 claude」行為請改設 `AGENT_RUNTIME=claude` 或設 `ACP_EXECUTABLE=claude-agent-acp`。

## Open Questions

- ~~**Q1：是否在本 change 同時加 codex CLI？**~~ **已決**：不加。先確保 OpenCode 與架構抽象 OK，codex 另開 change（runtime 結構應該已經夠表達）。
- **Q2：OpenCode ACP server 是否接受 `permissionMode: "bypassPermissions"` + `workspace_path`？** Phase 0 實測決定；若不支援，本 change 加最小映射（per-runtime `new_session` template），否則 D3 假設成立。
- **Q3：是否要在 Settings UI 顯示「目前實際執行的 ACP binary」（debug info）？** 暫不加，落到 startup log 即可（`ACP process started executable=...` 已存在）。
- **Q4：image 是否該透過 build flag 砍 runtime（compile-time pick）？** 不加，runtime-time 切換的彈性 > image 大小節省。
- **Q5：`ACP_EXECUTABLE_ARGS` 命名是否好？** 可以；如果有更好的名字以 D6 為準再調。
