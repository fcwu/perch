## Context

Perch 目前的 Discord integration 使用 PTY bridge 模式：每個 Discord channel 持有一個獨立的 `discordSession`，內含一個 PTY process 執行 Claude Code CLI。訊息處理流程為：

1. Discord 訊息 → `onMessage()`
2. 等待 PTY warm-up（偵測 "bypass permissions" 或 ❯ prompt）
3. 將訊息文字寫入 PTY stdin
4. Claude Code 透過 `/hook` endpoint 回報事件（PreToolUse / PostToolUse / Stop）
5. Stop event 帶回最終回應文字，送回 Discord

PTY bridge 的問題：
- **暖機偵測脆弱**：依賴解析特定 prompt 字串（"bypass permissions"、❯）
- **終端輸出解析**：raw stdout 混雜 ANSI codes，非結構化
- **Permission 阻塞**：非 `--trust-all-tools` 模式下 Claude Code 等待互動確認
- **難以測試**：PTY 行為難以在 unit test 中模擬

Agent Communication Protocol（ACP）提供標準化的 JSON-RPC over stdio 協議讓外部系統與 agent 互動。`claude-agent-acp`（`@agentclientprotocol/claude-agent-acp`）是官方的 Claude Code ACP 適配器，讓 Perch 可以用結構化 JSON-RPC 直接與 Claude Code 通訊，完全取代脆弱的 PTY 橋接。

## Goals / Non-Goals

**Goals:**
- 以 ACP JSON-RPC over stdio 取代 Discord session 的 PTY 橋接層
- **Perch 自行管理** Claude Code subprocess（一個 channel 一個 process），不依賴外部 bridge service
- 移除 warm-up 等待、PTY 寫入、終端輸出解析邏輯
- 保留 per-channel 對話上下文（session 跨多輪訊息持續存在）
- 支援 `permissionMode: "bypassPermissions"` 取代 `--trust-all-tools` flag
- 保留所有 Discord 輸出格式邏輯（訊息分割、emoji、CJK 對齊）
- 保留 DM allowlist 與公開頻道 mention 驗證

**Non-Goals:**
- 依賴外部 ACP bridge service（如 aws-samples/sample-acp-bridge）
- 改寫 Telegram integration
- 移除 Web PTY（`/ws` endpoint）—— 僅 Discord session 改用 ACP
- 即時 streaming 編輯 Discord 訊息（保持完成後一次送出）

## Architecture

```
Before (PTY):
  Discord msg → discordSession → PTY process (claude CLI) → raw stdout parse → Discord reply

After (ACP stdio):
  Discord msg → discordSession → ACPProcess (claude-agent-acp) ←→ JSON-RPC stdio → Discord reply
                                    ↓
                              Claude Code SDK → Anthropic API
```

每個 Discord channel 對應一個 `ACPProcess`：
- Perch fork `claude-agent-acp` subprocess（stdin/stdout pipes）
- ACP handshake on start：`initialize` → `new_session`（帶 `permissionMode: "bypassPermissions"`）
- 每則訊息：`prompt(sessionID, text)` → 等待 `agent_message_chunk` callbacks → `RunCompleted`
- Subprocess crash 自動重啟，重新 initialize

## Decisions

### D1：per-channel ACPProcess，Perch 自行管理 subprocess

**決策**：每個 Discord channel 持有一個 `ACPProcess` struct，內含 `claude-agent-acp` subprocess（stdin/stdout pipes）。Perch 負責 subprocess 的啟動、重啟、idle 清理。

**理由**：
- 保留與 PTY 模式相同的 per-channel 對話上下文語義
- 不依賴外部 service，部署簡單（只有 Perch binary + claude-agent-acp npm 套件）
- ACP JSON-RPC 提供結構化通訊，無需 PTY warm-up 偵測

**與 PTY 的差異**：
- ❌ 舊：`exec.Command("claude", args...)` + PTY + raw stdout parse
- ✅ 新：`exec.Command("claude-agent-acp")` + stdin/stdout pipes + JSON-RPC

### D2：ACP JSON-RPC stdio 協議

**決策**：實作 ACP JSON-RPC over stdio client（`acp_process.go`），依序發送：
1. `initialize` request → 取得 server capabilities
2. `new_session` request（帶 `permissionMode: "bypassPermissions"`）→ 取得 `sessionID`
3. 每則訊息：`prompt` request（帶 `sessionID` + text）→ 收集 `agent_message_chunk` notifications → 等待 `RunCompleted` / `RunFailed`

**理由**：
- ACP 是標準化協議，相比 PTY raw output 更穩定
- JSON-RPC request/response correlation（用 ID matching）比 PTY streaming 更可靠
- 易於單元測試（mock subprocess stdin/stdout）

### D3：Hook endpoint 保留但不再是 Discord 必要路徑

**決策**：`/hook` endpoint 保留供 web PTY session 使用，Discord 改用 ACP event 取得完成通知，不再依賴 `/hook`。

**理由**：
- Web PTY session 仍需要 hook routing
- 避免 ACP + hook 雙重通知造成重複回覆

### D4：`claude-agent-acp` 執行檔設定

**決策**：透過環境變數 `ACP_EXECUTABLE`（預設 `claude-agent-acp`）指定 ACP subprocess 執行檔。`CLAUDE_CODE_EXECUTABLE` 由 `claude-agent-acp` 自行讀取，Perch 不需處理。

**相關環境變數**：

| 環境變數 | 說明 | 預設值 |
|---------|------|--------|
| `DISCORD_ACP_ENABLED` | 啟用 Discord ACP 模式（取代 PTY）| `false` |
| `ACP_EXECUTABLE` | ACP subprocess 執行檔路徑 | `claude-agent-acp` |
| `ACP_RUN_TIMEOUT` | 每則訊息的最長等待秒數 | `300`（5 分鐘）|
| `CLAUDE_CODE_EXECUTABLE` | 由 claude-agent-acp 讀取，覆寫 claude binary 路徑 | 自動偵測 |

> 注意：移除舊設計的 `ACP_BASE_URL`、`ACP_AGENT_NAME`、`ACP_TRUST_ALL_TOOLS`（不再需要）。

### D5：Permission 模式：`bypassPermissions`

**決策**：`new_session` 時傳入 `permissionMode: "bypassPermissions"`（須非 root 執行），取代 PTY 模式下的 `--trust-all-tools` flag。

**理由**：
- Discord bot 是非互動環境，所有工具呼叫須自動允許
- `bypassPermissions` 是 claude-agent-acp 的標準 API，比 `--trust-all-tools` flag 更乾淨

### D6：Subprocess 生命週期管理

**決策**：
- **啟動**：channel 第一則訊息時 lazy init subprocess（不預先啟動）
- **重啟**：subprocess exit 後下一則訊息觸發重啟（重新 `initialize` + `new_session`）
- **Idle cleanup**：超過 `ACP_SESSION_IDLE_TTL`（預設 24h）無訊息則關閉 subprocess 釋放資源

**理由**：
- Lazy init 避免啟動時建立大量閒置 process
- 自動重啟確保 crash recovery，對話上下文部分遺失但 session 可續用

### D7：`IMAdapter` 介面維持現有調整

**決策**：`IMAdapter.Start(cfg IMConfig)` 維持現有介面，`IMConfig.ACPClient` 欄位改名為 `IMConfig.ACPEnabled bool`（或直接用環境變數判斷）。

## Implementation Plan

### 新增 `acp_process.go`

```go
type ACPProcess struct {
    executable string
    workdir    string
    cmd        *exec.Cmd
    stdin      io.WriteCloser
    stdout     *bufio.Reader
    sessionID  string
    mu         sync.Mutex
    nextID     int
}

func NewACPProcess(executable, workdir string) *ACPProcess
func (p *ACPProcess) Start(ctx context.Context) error  // fork + initialize + new_session
func (p *ACPProcess) Prompt(ctx context.Context, text string) (string, error)
func (p *ACPProcess) Stop()
func (p *ACPProcess) IsRunning() bool
```

### 修改 `im_discord.go`

- `discordSession.acpClient *ACPClient` → `discordSession.acpProcess *ACPProcess`
- `handleWithACP()` → `handleWithACPProcess()`：呼叫 `acpProcess.Prompt()` 取得結果
- 移除 `StreamRun` / `CreateRun` 呼叫

### 移除

- `acp_client.go`（HTTP-based ACP client，整個方向不對）
- `acp_client_test.go`（對應 HTTP client 的測試）

### 修改 `im_discord_acp_test.go`

- mock subprocess（fake stdin/stdout）取代 mock HTTP server

## Risks / Trade-offs

- **ACP JSON-RPC 實作複雜度**：需自行在 Go 實作 JSON-RPC 2.0 over stdio，包含 ID correlation 與 notification handling。→ Mitigation：協議結構簡單，可參考 MCP Go client 實作模式

- **claude-agent-acp npm 依賴**：需要 Node.js 環境，部署時需確保 `claude-agent-acp` 可執行。→ Mitigation：`ACP_EXECUTABLE` 允許指定完整路徑；若不可用則 fallback PTY 模式

- **Session 重啟後上下文遺失**：subprocess crash 重啟後新 session，Claude Code 不記得之前的對話。→ Acceptable：與 PTY 模式 crash 行為一致

- **Idle subprocess 資源消耗**：長存的 subprocess 佔用記憶體。→ Mitigation：`ACP_SESSION_IDLE_TTL` 控制自動清理

## References

- [agentclientprotocol/claude-agent-acp](https://github.com/agentclientprotocol/claude-agent-acp)：官方 Claude Code ACP 適配器
- [agentclientprotocol/python-sdk](https://github.com/agentclientprotocol/python-sdk)：ACP 協議參考實作（stdio transport）
- [agentclientprotocol.com](https://agentclientprotocol.com/)：ACP 協議規格
