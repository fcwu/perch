## 1. Image：jq 套件

- [ ] 1.1 `Dockerfile` base layer 加 `apt-get install -y jq`（或對應 Alpine 的 `apk add jq`，視 image 而定）
- [ ] 1.2 在 image 內驗證 `which jq` 可用

## 2. cp host claude config 到 local 可寫副本

- [ ] 2.1 entrypoint.sh 偵測 `/etc/perch-claude-host/` 是否存在；存在則執行 `mkdir -p /home/perchuser/.claude && cp -a /etc/perch-claude-host/. /home/perchuser/.claude/`
- [ ] 2.2 cp 後對 D5' 的排除清單執行 `rm -rf`：`sessions/`、`projects/`、`cache/`、`debug/`、`backups/`、`shell-snapshots/`、`history.jsonl`
- [ ] 2.3 若 `/etc/perch-claude-host/` 不存在（fresh container），改為 `mkdir -p /home/perchuser/.claude`（空目錄），讓後續 D4 seed 流程處理初始化
- [ ] 2.4 cp 失敗（disk full、staging 路徑為檔案非目錄等）log warning 但不終止啟動
- [ ] 2.5 `tests/docker-compose.local-test.yml` mount 改為 `${HOME}/.claude:/etc/perch-claude-host:ro`
- [ ] 2.6 撰寫 / 更新 README 段落：說明新的 mount 慣例 (`${HOME}/.claude:/etc/perch-claude-host:ro`)、release note 標 breaking change 與升級指引

## 4. `.claude.json` onboarding flag seed

- [ ] 4.1 entrypoint.sh 啟動時偵測 `/home/perchuser/.claude.json`：不存在則建立空 `{}`
- [ ] 4.2 對下列欄位逐一檢查「不存在或為 null」→ 用 jq 補預設值：
  - `.hasCompletedOnboarding = true`
  - `.theme = "dark-daltonized"`（或 `"dark"`，二選一，需確認哪個是 Claude Code 2.1.x 的安全預設）
  - `.hasAcceptedAllTerms = true`
  - `.projects["$WORKDIR"].hasTrustDialogAccepted = true`
- [ ] 4.3 seed 後 chown 為 `${PUID}:${PGID}`（PUID 模式下）
- [ ] 4.4 已存在的欄位（包含 false 值）一律保留，僅補缺漏

## 5. 測試：fresh container

- [ ] 5.1 撰寫 `tests/test-container-bootstrap.md`（或加入 `tests/test-basic-startup.md`）測試案例：
  - **TBC01**：fresh 容器（無 `tests/test-perchuser/`、無 `tests/test-claude-rw/`，host `~/.claude:ro` 直 mount）能起、Discord PTY 第一句訊息有 reaction
  - **TBC02**：seed 已存在 `.hasCompletedOnboarding=true` 時不被覆寫
  - **TBC03**：tmpfs 掛載缺失時，entrypoint log warning 但仍啟動
  - **TBC04**：Claude Code Bash 工具能成功（驗證 session-env 寫得進去）
- [ ] 5.2 在 batch B QA cycle 跑 5.1 + 既有 6 個案例（MT12 / T55 / T56 / T19 / T27 / T33-forward），全綠才視為完成

## 6. 還原測試 fixture

- [ ] 6.1 `tests/docker-compose.local-test.yml`：mount 改為 `${HOME}/.claude:/etc/perch-claude-host:ro`（與 2.5 同步）
- [ ] 6.2 確認 `tests/test-claude-rw/`、`tests/test-perchuser/`、`tests/test-data/`、`tests/test-workspace/` 不再被 docker-compose mount 引用
- [ ] 6.3 `.gitignore` 加 `tests/test-data/`、`tests/test-workspace/`（runtime artifact，與 B 無關但這次一併處理）
- [ ] 6.4 commit message 引用本 change ID

## 7. 文件

- [ ] 7.1 `README.md` 新增 / 更新「Container 第一次啟動」段落，說明：
  - 新 mount 慣例：`${HOME}/.claude:/etc/perch-claude-host:ro`（entrypoint 會 cp 進容器 local 副本）
  - **Breaking change**：既有 deploy 從 `${HOME}/.claude:/home/perchuser/.claude:ro` 升級的步驟與理由
  - 排除清單（不會帶進容器的 host 子目錄與檔案）
  - 容器自動 seed `.claude.json` 必要 onboarding flag
- [ ] 7.2 `CLAUDE.md`（perch 專案層）的「容器除錯」段補一行「entrypoint seed log」如何辨識（grep `perch entrypoint:`）

## 8. Open Questions Resolution

- [ ] 8.1 決定 Q1：fresh container（無 host claude staging mount）時 entrypoint 行為？建議：mkdir 空 `~/.claude`，由 D4 seed 處理 `.claude.json`，使用者首次 `claude /login`。
- [ ] 8.2 決定 Q2：`PERCH_CLAUDE_EXCLUDE` 是否需要可配置？建議：不加，預設清單夠用。
