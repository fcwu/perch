## Why

Perch 目前把 AI agent runtime 寫死在 Claude Code：container 只安裝 `claude`、主 PTY 與 Discord PTY 都只會啟動 `claude`、project-level 設定注入也只支援 `.claude/settings.json`。repo 雖然已經有 `.opencode/` plugin 內容，但 OpenCode 還沒有被接進正式執行流程，因此使用者無法在 Perch 裡用 OpenCode，同時保留 Discord、schedule、hooks 與既有設定體驗。

## What Changes

- 新增可選的 agent runtime 設定，讓 Perch 可啟動 `claude` 或 `opencode`，而不是將 PTY 命令硬編碼為 `claude`
- 將主 PTY、Discord session PTY、scheduler 目標 PTY 的 runtime 啟動邏輯統一到同一個 runtime abstraction
- 新增 OpenCode runtime 的 image/runtime 準備流程，包含安裝可執行檔與必要的 home/workspace 設定
- 擴充 project-level 設定注入機制，讓 Claude 既有的 hooks/skills merge 模式可以套用到 OpenCode 對應的設定檔與 plugin 目錄
- 文件與測試更新，明確說明如何選擇 runtime，以及哪些 Discord / schedule / hook 行為在 Claude 與 OpenCode 間需要保持一致

## Capabilities

### New Capabilities
- `agent-runtime-selection`: 以設定選擇 Perch 啟動的 agent runtime，並讓 main PTY、Discord PTY、scheduler PTY 使用一致的 runtime 啟動規則
- `agent-runtime-integration`: 為不同 runtime 注入對應的 hooks、settings、skills 或 plugins，讓 Discord 通知、hook callback、排程與現有 project-level 設定模式在 OpenCode 下也能工作

### Modified Capabilities

## Impact

- `main.go`、`pty.go`、`im_discord.go`：不再直接假設執行命令一定是 `claude`
- `entrypoint.sh`、`Dockerfile`：新增 OpenCode 安裝與 runtime-specific 設定／plugin 複製邏輯
- `claude/` 旁可能新增 `opencode/` runtime 資產，或將現有設定 merge 流程泛化
- `README.md`、`docs/test-cases.md`：補充 runtime 選擇、掛載方式、OpenCode 使用說明與 parity 測試案例
