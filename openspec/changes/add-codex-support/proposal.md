## Why

`2026-05-01-multi-agent-acp-runtime` 把 `AgentRuntime.ACPExecutable` + `ACPArgs` 兩個欄位接通了 ACP 路徑，並明說：

> Phase 2（已決：不在本 change）：Codex CLI ACP 支援（`@openai/codex` + `codex acp`）— 留給後續 change，本 change 確保 runtime.go 設計能容納（`ACPExecutable + ACPArgs` 可表達任意 ACP-compatible CLI）。

現在補上 codex 的支援。Phase 2 的舊假設（`codex acp` 是 official subcommand）pre-flight 後**不成立**：official `@openai/codex` CLI 只有 `codex mcp`（Model Context Protocol，不是 ACP），不存在 `codex acp` 子指令。

實際 ACP 進入點是 **`@zed-industries/codex-acp`** — Zed 官方維護的 npm wrapper（v0.12.0），binary 名為 `codex-acp`，直接執行就 stdio JSON-RPC，不需 subcommand。`optionalDependencies` 已涵蓋 `linux-x64`、`linux-arm64`、`darwin-arm64/x64`、`win32-arm64/x64`，install 由 npm 依 host arch 自動挑 native binary，與 `claude-agent-acp` 相同 pattern（不需 `dpkg --print-architecture` 邏輯）。

> npm registry metadata（pre-flight 2026-05-01）：`@zed-industries/codex-acp@0.12.0`，`bin: codex-acp`，6 個 platform-specific optional deps 涵蓋 perch 的 amd64/arm64 部署。

加進來後 `AGENT_RUNTIME=codex` 可讓 chat-API、Discord、Telegram、web `/ws` 全部走 OpenAI Codex；既有 claude / opencode 部署完全不受影響。

## What Changes

- **修改** `runtime.go::loadAgentRuntime` 加 `case "codex"`：
  - `Name: "codex"`、`Command: "codex"`（給未來 `RunAgent` 子指令用，本 change 暫不實作 codex subagent mode）
  - `ACPExecutable: "codex-acp"`、`ACPArgs: nil`（無需 subcommand）
  - `ProjectConfigDir / ProjectConfigFile`：暫沿用 `.codex` / `config.toml`（codex CLI 慣例；不影響 ACP 流程）
  - `AssetDir: "/app/perch-codex"`、`SupportsHooks: false`
  - `ArgsEnv: "CODEX_ARGS"`（與 `CLAUDE_ARGS` / `OPENCODE_ARGS` 對齊）
- **修改** `runtime_test.go::TestLoadAgentRuntime_ACPFields` 加 codex sub-case：assert `ACPExecutable=="codex-acp"`、`ACPArgs==nil`
- **修改** Dockerfile：
  - `npm install -g` 那行加 `@zed-industries/codex-acp`（與既有 `@anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp` 同步驟，npm 自動依 host arch 抓對的 native 二進位，不需 dpkg arch detect）
  - 不動 opencode 的 `sst/opencode` tarball download（codex 不走那條路）
- **修改** `claude-container-bootstrap` 系列 entrypoint：若 `AGENT_RUNTIME=codex` 則 seed `/app/perch-codex/`（暫時可放空目錄；實際 codex 配置 future change 補）
- **修改** Settings UI (`SettingsPanel.tsx`)：`agent.runtime` RadioGroup 加 `codex` 選項（與 claude / opencode 並列）
- **修改** `.env.example`、`README.md`：
  - `AGENT_RUNTIME` 列 `claude` / `opencode` / `codex` 三選項
  - 「Agent Runtime」段落補一個 「Codex 額外注意」小節：`OPENAI_API_KEY` 必填、auth flow（API key 不需 OAuth）、`codex-acp` 是 Zed 維護的 wrapper
- **新增** 測試 `tests/test-codex-runtime.md`：CX01（codex default baseline）/ CX02（chat-API 切到 codex spawn `codex-acp`）/ CX03（OPENAI_API_KEY 缺失時 graceful error）/ CX04（ACP_EXECUTABLE override 仍然吃）
- **新增** Phase 0 pre-flight task：實測 `codex-acp` stdio JSON-RPC（`initialize` / `session/new` / `session/prompt` 對 perch 既有 payload 接受度）

## Capabilities

### Modified Capabilities

- `agent-runtime-selection`：`loadAgentRuntime` 接受 `AGENT_RUNTIME=codex`，回 `AgentRuntime{Name:"codex", ACPExecutable:"codex-acp", ACPArgs:nil}`
- `agent-runtime-integration`：runtime image 預裝 `codex-acp`（npm install），與 claude-agent-acp + opencode 共存
- `acp-client`：codex runtime 的 ACP 行為（`codex-acp` 直接執行、env-only auth）

## Out of Scope

- Codex subagent 非互動模式（`codex exec` / `RunAgent`）— web `/ws` PTY 用 codex 的 case 留待 future change
- Codex 專屬的 system prompt / tool catalogue 客製（先吃 default）
- `~/.codex/` config seed（image 不預塞，sysadmin 自掛 volume 即可）
- 多 runtime 同時跑（一個 perch instance 仍只認一個 `AGENT_RUNTIME`）
- ACP `set_mode` 對 codex 的相容性 — 若 codex-acp reject `bypassPermissions` 也照 opencode 既有 pattern：graceful warning-and-continue（`acp_process.go:189` 已涵蓋）
