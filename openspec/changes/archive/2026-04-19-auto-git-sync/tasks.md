## 1. 環境變數與設定

- [x] 1.1 定義 `WORKSPACE_GIT_SYNC_ENABLED`、`WORKSPACE_GIT_SYNC_INTERVAL`、`WORKSPACE_PATH`、`WORKSPACE_GIT_TOKEN`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` 環境變數讀取邏輯
- [x] 1.2 撰寫 `WorkspaceSyncConfig` struct，包含 `Enabled bool`、`Interval time.Duration`、`WorkspacePath string`、`GitToken string`、`NotifyChannelID string`
- [x] 1.3 撰寫 `LoadSyncConfig()` 函式，從環境變數載入並設定預設值（interval=60s, enabled=false, path=/workspace）

## 2. Git 工具函式

- [x] 2.1 實作 `isGitRepo(path string) bool`：偵測 `<path>/.git` 是否存在
- [x] 2.2 實作 `isDirty(path string) (bool, error)`：執行 `git status --porcelain` 判斷是否有未提交變更
- [x] 2.3 實作 `isRebasing(path string) bool`：偵測 `.git/rebase-merge` 或 `.git/rebase-apply` 是否存在
- [x] 2.4 實作 `runGit(ctx context.Context, path string, args ...string) (string, error)`：統一的 git 指令執行函式，捕捉 stdout/stderr

## 3. Git Credential 注入

- [x] 3.1 實作 `injectGitToken(token string, remoteURL string, logger *slog.Logger) error`：偵測 remote URL 是否為 HTTPS
- [x] 3.2 HTTPS remote：log `injecting git token for host <host>`（不 log token 值）→ 寫入 `~/.git-credentials` → 執行 `git config --global credential.helper store` → log 每步結果
- [x] 3.3 SSH remote：log Warning `git token ignored for SSH remote`，不注入
- [x] 3.4 在啟動時（`StartWorkspaceSync` 前）呼叫 `injectGitToken`，token 為空則 log info 並 skip

## 4. Sync 核心邏輯

- [x] 4.1 實作 `syncOnce(ctx context.Context, cfg WorkspaceSyncConfig, notify NotifyFunc) error`：單次 sync 流程（stash → pull --rebase → stash pop → push），每個步驟呼叫前後均記錄 slog
- [x] 4.2 rebase 衝突：log Warn（偵測）→ 執行 abort → log abort stdout+stderr（Info）→ log 結果（Info/Error）→ 呼叫 notify
- [x] 4.3 stash pop 失敗：log Error（含 stash ref + 完整 stderr）→ skip push → 呼叫 notify
- [x] 4.4 push 失敗：log Error（含完整 stdout+stderr）→ 呼叫 notify，不 panic
- [x] 4.5 實作 debounce：同類型錯誤 5 分鐘內只 notify 一次（`map[string]time.Time` 記錄上次通知時間）

## 5. Discord 通知整合

- [x] 5.1 在 `DiscordSessionManager` 新增 `SendToChannel(channelID string, msg string) error` 方法：直接呼叫 `dgo.ChannelMessageSend(channelID, msg)`（與 `allowedChannelID` 無關）
- [x] 5.2 在 `IMManager` 新增 `SendText(msg string) error`：遍歷所有 adapter，若 adapter 實作 `TextSender` interface 則呼叫

## 6. 背景 Goroutine 整合

- [x] 6.1 新增 `workspace_sync.go` 檔案，包含上述所有函式
- [x] 6.2 定義 `NotifyFunc func(errType string, msg string)` 型別
- [x] 6.3 實作 `StartWorkspaceSync(ctx context.Context, cfg WorkspaceSyncConfig, notify NotifyFunc)`：啟動 ticker loop（notify 為 nil 時只 log）
- [x] 6.4 在 `main.go` 初始化階段：呼叫 `injectGitToken`，再用 `discord.SendToChannel(cfg.NotifyChannelID, msg)` 包成 `NotifyFunc`（`NotifyChannelID` 為空時 notify = nil）傳入 `StartWorkspaceSync`
- [x] 6.5 確保 goroutine 在 `ctx.Done()` 時優雅退出

## 7. 測試

- [x] 7.1 撰寫 `isGitRepo` 單元測試（valid repo / non-repo）
- [x] 7.2 撰寫 `isDirty` 單元測試（clean / dirty state）
- [x] 7.3 撰寫 `isRebasing` 單元測試（no rebase / rebase-merge exists）
- [x] 7.4 撰寫 `syncOnce` 整合測試（使用 temp git repo，測試 clean sync 流程）
- [x] 7.5 撰寫 `LoadSyncConfig` 單元測試（各環境變數組合，含 GIT_TOKEN）
- [x] 7.6 撰寫 `injectGitToken` 單元測試（HTTPS remote / SSH remote / token 空白 / log 不含 token）
- [x] 7.7 撰寫 debounce 單元測試：同類型錯誤在 5 分鐘內第二次不觸發 notify
- [x] 7.8 撰寫 `DiscordSessionManager.SendToChannel` 單元測試（dgo nil 時）
