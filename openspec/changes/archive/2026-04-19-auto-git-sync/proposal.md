## Why

當多個 agent（如 Discord bot、OpenCode agent）或使用者在不同機器上共用同一個 git workspace 時，需要頻繁手動 pull/push 才能保持同步，容易發生衝突或遺漏更新。透過定時自動 sync，可以讓 workspace 隨時保持最新狀態，降低人工介入成本。

## What Changes

- 新增 `auto-git-sync` 功能：偵測到 `/workspace` 是 git repo 時，每 60 秒自動執行 pull + push
- 支援設定 sync interval（預設 60 秒）
- 支援設定目標 remote/branch（預設 `origin` / 當前 branch）
- 衝突發生時記錄 error log，不強制覆蓋，並透過 Discord 通知使用者手動解決
- 支援 `WORKSPACE_GIT_TOKEN` 環境變數，perch 自動設定 git credential，免手動設定 SSH key
- 可透過環境變數或設定檔啟用/停用

## Capabilities

### New Capabilities

- `workspace-git-sync`: 定時自動偵測 workspace 是否為 git repo，並執行 pull + push 同步邏輯，包含衝突偵測與 error 回報

### Modified Capabilities

（無）

## Impact

- **程式碼**：新增 `workspace_sync.go`（或對應模組），整合進 perch runtime 啟動流程
- **設定**：新增 `WORKSPACE_GIT_SYNC_INTERVAL`、`WORKSPACE_GIT_SYNC_ENABLED`、`WORKSPACE_GIT_TOKEN`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` 環境變數
- **依賴**：使用系統 `git` 指令（shell out），無額外 library 依賴
- **副作用**：若 workspace 有未提交變更（dirty），sync 前自動 stash 或跳過，避免資料遺失
