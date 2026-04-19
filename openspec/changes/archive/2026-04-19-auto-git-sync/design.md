## Context

Perch 目前支援多種 agent runtime（Discord bot、OpenCode agent 等），這些 agent 可能在不同機器或容器中對同一個 `/workspace` git repo 進行讀寫。目前沒有任何自動同步機制，使用者需要手動 pull/push，導致 agent 取得的資料可能過時，也容易累積未同步的本地變更。

## Goals / Non-Goals

**Goals:**
- 在 perch 啟動時偵測 `/workspace` 是否為 git repo
- 若是，啟動背景 goroutine 定時執行 `git pull --rebase` + `git push`
- 支援透過環境變數設定 interval 與啟用/停用
- 遇到衝突或 push 失敗時，記錄 error、保持現有狀態（不強制覆蓋），並透過 Discord 通知
- 支援 `WORKSPACE_GIT_TOKEN` 環境變數，透過 git credential helper 注入 token
- 若 workspace 為 dirty（有未提交變更），sync 前先 stash，sync 後 stash pop

**Non-Goals:**
- 不支援多 remote 同步
- 不自動 commit 未提交的變更（stash 僅用於暫存以利 pull）
- 不處理 submodule 遞迴同步
- 不提供 UI 或 API 端點來手動觸發 sync

## Decisions

### D1：Shell out vs. go-git library

**選擇**：Shell out（`exec.Command("git", ...)`）

**理由**：
- go-git 不完整支援所有 git 功能（如 rebase、credential helper）
- 系統 git 已在 perch 的部署環境中可用
- Shell out 實作簡單，行為與使用者手動操作一致
- 替代方案（go-git）：需額外依賴，且 rebase 支援不穩定

### D2：衝突處理策略

**選擇**：Abort + log，不強制覆蓋

**理由**：
- workspace 可能有 agent 正在使用的重要資料，強制覆蓋風險高
- 讓使用者手動解決衝突比靜默覆蓋更安全
- 替代方案（force push / ours strategy）：可能造成資料遺失，不可接受

### D3：Dirty workspace 處理

**選擇**：`git stash` 暫存 → pull → `git stash pop`

**理由**：
- 允許在有本地變更的狀態下仍能 pull 最新遠端變更
- stash pop 失敗時保留 stash，不會遺失資料
- 替代方案（直接 pull 忽略 dirty）：pull --rebase 會拒絕執行

### D5：Git Credential 注入

**選擇**：透過 `git config --global credential.helper store`，並在啟動時將 `WORKSPACE_GIT_TOKEN` 寫入 `~/.git-credentials`（格式：`https://x-token-auth:<token>@<host>`）

**理由**：
- 不需要修改 git remote URL
- 支援 GitHub / GitLab / Bitbucket 等主流 HTTPS remote
- 替代方案（每次 git 指令帶 `-c credential.*`）：token 出現在 process args，可被 `ps` 看到（安全疑慮）
- 替代方案（SSH key 掛載）：需容器 volume mount，不適合純環境變數設定

**Log 要求**：每個步驟都需要 log：
- `workspace_sync: injecting git token for host <host>` （token 值不得出現在 log）
- `workspace_sync: git credential helper set to store`
- `workspace_sync: credential injection complete`
- 失敗時：`workspace_sync: credential injection failed: <err>`

**限制**：僅支援 HTTPS remote；SSH remote 繼續依賴系統 SSH key。

### D6：Discord 失敗通知

**選擇**：在 `DiscordSessionManager` 新增 `SendText(msg string) error` 方法，直接呼叫 `dgo.ChannelMessageSend(allowedChannelID, msg)`；sync loop 注入一個 `NotifyFunc` callback。

**目標 Channel**：`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL`（新增獨立 env var，填入 Discord channel ID）。未設定時只 log，不發 Discord 通知。

**理由**：
- `DISCORD_CHANNEL_ID` 是 bot 的訊息 filter（控制收哪個 channel 的訊息），語意與「通知目的地」不同，不可混用
- 獨立 env var 允許使用者把 git sync 通知送到不同 channel（如專屬的 #infra-alerts）
- `DiscordSessionManager` 已持有 `dgo`，在 `SendText` 裡直接用 `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` 呼叫 `ChannelMessageSend`
- `NotifyFunc` 型別讓 sync loop 解耦：不直接依賴 Discord，notify 為 nil = 只 log

**驗證手法**：
1. 設定 `WORKSPACE_GIT_SYNC_ENABLED=true`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=<channel_id>` 並準備一個 bare remote repo
2. 人為製造衝突（在遠端和本地修改同一行）
3. 等待下一個 sync tick，確認：
   - `slog` 輸出有 `workspace_sync: rebase conflict detected`、abort 指令輸出、abort 結果三筆 log
   - 指定 channel 收到 Discord 訊息，內容包含 `⚠️ git sync conflict`
4. 若 `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` 未設定，確認只有 slog，無 Discord 訊息

**通知時機**：
1. Rebase conflict abort → 通知一次
2. Stash pop conflict → 通知一次
3. Push rejected → 通知一次（debounce：同類型 5 分鐘內不重複）

### D4：整合點

**選擇**：在 `main.go` 的 workspace 初始化階段後，以 goroutine 啟動 sync loop

**理由**：
- 與現有啟動流程最小侵入
- goroutine 透過 context cancel 可優雅關閉

## Risks / Trade-offs

- **Git credential 問題** → Mitigation：支援 `WORKSPACE_GIT_TOKEN` 注入（詳見 D5）；若未設定則依賴系統 credential helper；sync 失敗時 log 並 Discord 通知
- **Rebase 衝突後 repo 進入 REBASING 狀態** → Mitigation：偵測到 `.git/rebase-merge` 或 `.git/rebase-apply` 存在時，執行 `git rebase --abort`；log 包含：偵測到衝突、abort 指令輸出、abort 結果（成功/失敗）
- **Stash pop 衝突** → Mitigation：stash pop 失敗時保留 stash（不 abort），log 包含：stash ref、錯誤輸出，通知使用者手動 `git stash pop`
- **頻繁 push 觸發 CI/CD** → Mitigation：目前不處理，使用者應自行評估 remote repo 的 CI 設定；未來可加 `--no-trigger` commit option

## Logging Requirements

所有 git 操作都必須有 before/after log，格式為 `slog` structured log：

| 時機 | Level | 訊息範例 |
|------|-------|---------|
| sync tick 開始 | Debug | `workspace_sync: starting sync` |
| isRebasing 為 true | Warn | `workspace_sync: rebase in progress, aborting` |
| git rebase --abort 輸出 | Info | `workspace_sync: rebase abort output: <stdout+stderr>` |
| rebase abort 結果 | Info/Error | `workspace_sync: rebase abort succeeded` / `failed: <err>` |
| isDirty 為 true | Debug | `workspace_sync: dirty workspace, stashing` |
| git stash 輸出 | Debug | `workspace_sync: stash output: <output>` |
| git pull --rebase 輸出 | Info | `workspace_sync: pull output: <output>` |
| pull 失敗 | Error | `workspace_sync: pull failed: <err> output: <output>` |
| git stash pop 輸出 | Debug | `workspace_sync: stash pop output: <output>` |
| stash pop 失敗 | Error | `workspace_sync: stash pop failed, stash ref: <ref>, err: <err>` |
| git push 輸出 | Info | `workspace_sync: push output: <output>` |
| push 失敗 | Error | `workspace_sync: push failed: <err> output: <output>` |
| sync 完成 | Debug | `workspace_sync: sync complete` |
| credential injection | Info/Error | 詳見 D5 |

## Migration Plan

1. 新功能預設**停用**（`WORKSPACE_GIT_SYNC_ENABLED=false`）
2. 使用者顯式設定 `WORKSPACE_GIT_SYNC_ENABLED=true` 才啟動
3. 無需 DB migration 或 API 版本變更

## Open Questions

- interval 60 秒是否合理？可能需要根據實際使用情境調整預設值
- Debounce 失敗通知的視窗應該多長？目前設計為「同一類型錯誤 5 分鐘內只通知一次」
