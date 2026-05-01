## Why

Perch 的 `AGENT_RUNTIME` 抽象（`runtime.go::loadAgentRuntime` 支援 `claude` / `opencode`）在 2026-04-30 `consolidate-acp-runtime` 把 chat-API、Discord、Telegram 統一走 ACP 之後，runtime 切換**只剩 web `/ws` PTY 一條路徑生效**：

- `acp_process.go::ACPProcess` 起 subprocess 時讀 `ACP_EXECUTABLE` env var（預設 `claude-agent-acp`）— 與 `AGENT_RUNTIME` **完全解耦**
- 設 `AGENT_RUNTIME=opencode` 後 chat-API / Discord / Telegram 仍然 spawn `claude-agent-acp`，使用者拿到 Claude 不是 OpenCode
- README 與 Settings UI 的「OpenCode runtime」選項實質誤導使用者

而 [sst/opencode](https://opencode.ai/docs/acp/) 自身已內建 ACP server (`opencode acp`)；同一條 ACP 協定也支援 OpenAI **codex CLI** 與其他 agents（`vscode-acp` README 列了 Claude、Codex、Copilot、Qwen、Gemini、OpenCode、Kiro 等）。換句話說 ACP 已成跨 agent 標準，perch 沒理由綁死 Claude。

額外問題：`Dockerfile:26-28` 從 `anomalyco/opencode` 抓 `linux-arm64.tar.gz` 寫死 — x86 host 起容器後 `opencode` binary 直接 EXEC error（`docker exec perch-local-test opencode --help` → `sh: 1: opencode: Exec format error`），目前 web `/ws` PTY 的 OpenCode 路徑在 amd64 部署也是壞的。

## What Changes

**Phase 1 — 補 OpenCode（本 change 主要工作）**：

- **修改** `runtime.go::AgentRuntime` 加 `ACPExecutable string` 與 `ACPArgs []string` 欄位
  - `claude` → `ACPExecutable: "claude-agent-acp"`, `ACPArgs: []`
  - `opencode` → `ACPExecutable: "opencode"`, `ACPArgs: []string{"acp"}`（`opencode acp` 開 ACP server）
- **修改** `acp_process.go::NewACPProcess` 接受 runtime-driven executable + args，不再讀 `ACP_EXECUTABLE` env 也不再 default `claude-agent-acp`
  - `ACP_EXECUTABLE` env var 改成 override（保留向下相容，給 dev 自製 fork 用）
- **修改** `acp_session_pool.go` 把 runtime-aware executable 注入每個 session
- **修改** Dockerfile：
  - 從 `anomalyco/opencode` 換成 `sst/opencode`（official）
  - tar 名從寫死 `linux-arm64` 改成 detect `dpkg --print-architecture` 對應 amd64/arm64 二選一（`linux-x64.tar.gz` 或 `linux-arm64.tar.gz`）
  - 同時保留 `claude-agent-acp` npm install（兩個 runtime 共存）
- **修改** `im_discord.go::DiscordSessionManager` 從 `acpExecutable string` 改成持有 runtime；起 ACP session 時把 runtime 傳進 pool
- **修改** Settings UI 的 Agent Runtime 描述：移除「OpenCode 限 web /ws」警語（已不再為真）；補一行「切 runtime 後 chat-API/Discord/Telegram 都會跟著切，需重啟」
- **新增** 測試：
  - `tests/test-multi-agent-runtime.md`：MR01-04（claude default / opencode 切換 / 切換後 chat-API runtime confirm / web `/ws` runtime confirm）
  - `runtime_test.go`：unit test 驗 `loadAgentRuntime` 對 claude/opencode 各自返回正確 ACPExecutable + ACPArgs

**Phase 2（已決：不在本 change）**：

- Codex CLI ACP 支援（`@openai/codex` + `codex acp`）— 留給後續 change，本 change 確保 runtime.go 設計能容納（`ACPExecutable + ACPArgs` 可表達任意 ACP-compatible CLI）

**不在範圍**：

- runtime per-conversation 切換（現在是 process 層級，要切換得重啟）
- 多 agent 同時跑（一個 perch instance 只認一個 `AGENT_RUNTIME`）
- ACP-incompatible agent runtime（Aider、Continue 等）— 那是另一個 protocol abstraction 的問題

## Capabilities

### Modified Capabilities

- `agent-runtime-selection`：`AgentRuntime` 結構擴充 `ACPExecutable` + `ACPArgs`；增加「ACP path 讀 runtime 而非 env」requirement
- `acp-client`：subprocess executable 由 runtime 決定，不再 hardcode `claude-agent-acp`
- `agent-runtime-integration`：image 預裝 binary 列表新增 sst/opencode；arch 自適應
