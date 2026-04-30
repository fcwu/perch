## Why

2026-04-30 batch B QA cycle 揭露 Claude Code 2.1.x 在 perch container 內的 3 個相容性問題，目前是用 test-side workaround 堵住（`tests/test-claude-rw/` RW copy、`tests/test-perchuser/.claude.json` 預先 jq 設 onboarding flag），但同樣的坑 production users 第一次起 perch container 也會踩——entrypoint.sh 沒處理 fresh `~/.claude.json` 與 user-level hooks，使用者會看到 Discord PTY 沒反應、Bash tool 工具失敗、interactive Claude 卡 theme dialog。

把這三件事在 entrypoint / image 層從根上修掉之後，測試端的 fixture 與 docker-compose mount workaround 都可以還原成乾淨的 `${HOME}/.claude:ro`，使用者第一次拉 image 也能順利跑起來。

對應 QA 報告：`tests/test-report-2026-04-30-1236-summary.md`「給工程的觀察與建議」段。

## What Changes

- **修改** mount 慣例：使用者把 host `~/.claude` 改 mount 到 staging 路徑 `/etc/perch-claude-host:ro`（非直接 mount 到 `/home/perchuser/.claude`），entrypoint.sh 啟動時 `cp -a /etc/perch-claude-host/. /home/perchuser/.claude/` 製成容器 local 可寫副本，避開 Claude Code 2.1.x rename `plugins/*.bak` 與 mkdir `session-env/<uuid>/` 的 EROFS
- **新增** entrypoint.sh 偵測 `~/.claude.json` 缺 `hasCompletedOnboarding` / `theme` / `hasAcceptedAllTerms` / `projects."<workspace>".hasTrustDialogAccepted` 任一個 flag 時自動 jq seed 預設值，避免 fresh 容器卡 theme dialog
- **修改** `tests/docker-compose.local-test.yml` 改用新 mount 慣例（`${HOME}/.claude:/etc/perch-claude-host:ro`），並刪除測試端 fixture 依賴
- **新增** image 預裝 `jq`（entrypoint seed onboarding flag 需要）

**註**：本 change 不涵蓋 user-level hooks merge — interactive Claude PTY 載 hook 的需求由 `consolidate-acp-runtime` change 處理（chat-API + IM 全面 ACP 化後，hook 系統整個移除，user-level merge 也不再需要）。

## Capabilities

### New Capabilities
- `claude-container-bootstrap`：Claude Code 在 perch container 內第一次啟動的環境準備（volatile dir overlay、user-level hooks merge、`.claude.json` onboarding flag seed），讓 RO mount 與 fresh fixture 都能順利運作

### Modified Capabilities
- 無（entrypoint 行為過去未在任何 capability 形式化，`agent-runtime-integration` 描述的是 runtime 選擇與 PTY 介接，不重疊）

## Impact

- `entrypoint.sh`：主要改寫對象，新增兩個 init 步驟（cp host claude → local writable、`.claude.json` seed）
- `Dockerfile`：base image 加 `jq`（不需 `rsync`，使用 `cp -a`；不需 SYS_ADMIN）
- `tests/docker-compose.local-test.yml`：mount 由 `${HOME}/.claude:/home/perchuser/.claude:ro` 改為 `${HOME}/.claude:/etc/perch-claude-host:ro`
- `README.md` / `docker-compose.yml`（產品端）：mount 慣例變動需要 release note 與升級指引（**breaking change**：既有 deploy 的 compose 必須更新）
- `tests/.gitignore`（或根 `.gitignore`）：加 `tests/test-data/`、`tests/test-workspace/` runtime artifact ignore
- 環境變數：無新增
- 文件：`README.md` 或 `CLAUDE.md` 新增「container 第一次啟動會 cp host claude config 進 local 可寫副本，並自動 seed `.claude.json`」說明
