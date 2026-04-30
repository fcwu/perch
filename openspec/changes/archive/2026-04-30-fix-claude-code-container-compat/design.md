## Context

perch container 啟動流程目前由 `entrypoint.sh` 統一處理：以 `PUID/PGID` 切到 `perchuser`、把 perch 內建 skill 複製到 `$WORKDIR/.claude/skills/`、以 `claude/merge-settings.js` 把 hooks 合進 `$WORKDIR/.claude/settings.json`，最後 `exec /app/perch`。Claude Code 自身的 config 由使用者掛 `~/.claude` 進來（典型 docker-compose 寫法 `${HOME}/.claude:ro`），entrypoint 從不碰 `~/.claude` 內容。

Claude Code 2.1.x 的初始化與 runtime 行為相對 1.x 有三處改變，與 perch 既有的容器設計衝突：

1. **`session-env/<uuid>/` per-tool-call mkdir**：每次 Bash / Edit / Read tool 執行都會在 `~/.claude/session-env/` 底下 mkdir 一個 UUID 目錄寫入 env 環境變數。如果 `~/.claude` 是 RO bind mount，整路徑寫入失敗 → tool 報錯 → Bash 工具完全用不了，但 perch 看不到友善的錯誤，只看到 ACP run / chat session 卡住或失敗。
2. **`plugins/*.bak` 啟動 rename**：`claude` 一啟動就 rename `~/.claude/plugins/<x>.bak`，RO mount 同樣 EROFS。
3. **interactive trust dialog**：`claude --permission-mode bypassPermissions --name discord:<channel>`（perch Discord PTY 的 spawn 方式）會跑一段 trust 確認流程，project-level `.claude/settings.json` 的 hooks **不會被載入**（即使預先在 `.claude.json` 標 `hasTrustDialogAccepted=true`）。`claude -p`（chat-API 模式）才會載 project hooks。
4. **fresh `.claude.json` 卡 theme dialog**：第一次起 container 時 `~/.claude.json` 缺 `hasCompletedOnboarding`，interactive claude 進主畫面前會卡 theme 選擇對話框，導致 PTY 第一句沒任何反應。

QA 報告 `tests/test-report-2026-04-30-1236-summary.md` 詳細描述了驗證過程；目前測試是用 `tests/test-claude-rw/`（host `~/.claude` 排除 volatile dir 的 RW copy）+ jq 預先 seed `tests/test-perchuser/.claude.json` 兩個 fixture 堵住，但**這只解測試**，production user `docker run` fresh 容器仍會踩同一個坑。

## Goals / Non-Goals

**Goals:**

- 容器內 Claude Code 2.1.x 在 RO bind-mounted `~/.claude` 上仍能正常運作（Bash 工具、plugin rename）
- interactive Claude PTY（Discord 模式）能讀到 perch hooks 並回 reaction
- fresh container 第一次跑時，`.claude.json` 自動 seed 必要 onboarding flag，使用者不需事前互動
- `tests/docker-compose.local-test.yml` 還原 `${HOME}/.claude:ro`、不需要 `tests/test-claude-rw/` 與 `tests/test-perchuser/.claude.json` fixture
- batch B 全套（MT12 / T55 / T56 / T19 / T27 / T33-forward）仍 PASS

**Non-Goals:**

- 改寫 Claude Code 自身行為（trust dialog、session-env 寫入）
- 改寫 perch agent runtime 抽象（`agent-runtime-integration` 不動）
- 處理 Claude Code 1.x → 2.x 的歷史升級（假設 user 從乾淨 fresh container 開始）
- 重新設計 IM hook 路由（hook routing 由 ACP / hook 既有實作維持）
- 把 host `~/.claude/sessions`、`projects` 等敏感資料隔離成「測試專用 sanitized copy」（這是 fixture 的決策，不是 entrypoint 該管的）

## Architecture

```
host ~/.claude (RO bind mount)──────► /etc/perch-claude-host (RO staging in container)
                                         │
                                         │ entrypoint.sh: cp -a /etc/perch-claude-host/. /home/perchuser/.claude/
                                         ▼
                                      /home/perchuser/.claude/  (LOCAL writable copy)
                                         ├── settings.json   (RW, claude can mutate)
                                         ├── plugins/        ◄─── claude rename *.bak → OK (writable)
                                         ├── session-env/    ◄─── claude mkdir <uuid>/ → OK (writable)
                                         └── ...

                                      /home/perchuser/.claude.json (LOCAL writable)
                                         ▲
                                         │ entrypoint jq-seed if missing onboarding flags

                                      /workspace/.claude/settings.json (project-level)
                                         ▲
                                         │ entrypoint merge perch hooks (existing — kept until consolidate-acp-runtime
                                         │ removes the hook system entirely)
```

**啟動順序（entrypoint.sh 修改後）：**

1. PUID/PGID 切換、`HOME=/home/perchuser`（既有）
2. `$WORKDIR/.perch` 建立 + chown（既有）
3. **NEW**：若 `/etc/perch-claude-host/` 存在（RO mount），`mkdir -p /home/perchuser/.claude && cp -a /etc/perch-claude-host/. /home/perchuser/.claude/`，產生容器 local 可寫副本
4. perch-bundled skills 複製到 `$WORKDIR/.claude/skills/`（既有）
5. **NEW**：`.claude.json` onboarding flag seed —— 對 `/home/perchuser/.claude.json` 用 jq 檢查 `hasCompletedOnboarding`、`theme`、`hasAcceptedAllTerms`、`projects."$WORKDIR".hasTrustDialogAccepted` 是否缺漏，缺則注入預設值
6. （既有）當 `DISCORD_BOT_TOKEN` 或 `TELEGRAM_BOT_TOKEN` 存在時，`claude/merge-settings.js` 跑一次 `PERCH_MERGE_TARGET=$WORKDIR/.claude/settings.json`。**註**：本步驟在 `consolidate-acp-runtime` change 完成後將整個移除（hook 系統會被全部移除）
7. chown -R `/home/perchuser/.claude` 與 `/home/perchuser/.claude.json`（PUID 模式下，cp / merge / seed 都是 root 跑的）
8. exec perch（既有）

## Decisions

### D1：entrypoint cp -a host claude config 到 local 可寫副本

**決策**：使用者改把 host `~/.claude` mount 到 staging 路徑 `/etc/perch-claude-host:ro`（不直接 mount 到 `/home/perchuser/.claude`）。entrypoint.sh 啟動時 `cp -a /etc/perch-claude-host/. /home/perchuser/.claude/` 製作容器 local 可寫副本。Claude Code 之後所有寫入（rename `plugins/*.bak`、mkdir `session-env/<uuid>/`、改 settings.json）都在 local 副本上，不影響 host。

**替代方案：**

- *tmpfs overlay 在 RO mount 上*：mount 兩個 tmpfs 到 `~/.claude/plugins` 與 `~/.claude/session-env`。技術可行但需要 docker-compose `tmpfs:` 宣告 / `--tmpfs` 旗標 / 視情況 `--cap-add SYS_ADMIN`。較大權限、宣告位置散落，使用者要記得加。
- *Named volume*：把 `/home/perchuser/.claude` 用 named volume 覆蓋，啟動時從 RO source 複製進去。本質與 cp 相同，多了一層 docker volume 管理。
- *symlink + 局部 cp*：non-volatile 檔 symlink 到 RO staging，只 `plugins/` cp + `session-env/` mkdir。少約 80% 磁碟，但 symlink-following 邊角案例多、實作複雜。
- *改 host mount 為 RW*：要求使用者直接 mount `~/.claude:rw`。違背「保護 host config」初衷，否決。

**理由**：

- 不需要 tmpfs / SYS_ADMIN / volume 管理，純 sh + cp，幾行就解決
- 副本與 host 隔離：容器跑爛了不傷 host config；也不會把容器內的 session-env 垃圾倒回 host
- 磁碟成本可控：典型 host `~/.claude` 不含 sessions/projects/cache 等個資/快取的話約 5–20 MB；含的話 50–200 MB。後者太大時可在 entrypoint 內 `cp` 後加 `rm -rf` 排除清單（見 D5'）
- 重啟容器自動拿 host 最新 credentials（refresh）

**取捨**：

- host `~/.claude` 在容器啟動後的變更（例如 host 上 `claude` re-auth 換 token）不會即時反映進容器；要重啟容器才同步。對 perch 使用情境（容器長跑、host 不常 re-auth）可接受
- `cp -a` 拷貝的 `.credentials.json` 進容器後，容器內的 process 都讀得到。若使用者想隔離敏感檔，靠 D5' 的排除清單

### D5'：cp 排除清單（取代舊 D5 的 tmpfs 宣告）

**決策**：entrypoint.sh `cp -a` 之後，對 `/home/perchuser/.claude/` 內已知的 volatile / 個資 / 快取 子目錄執行 `rm -rf`，免得把 host 不該進容器的東西帶進去。預設排除：

- `sessions/`（host 的歷史對話 session 紀錄）
- `projects/`（host 各 project 的歷史）
- `cache/`、`debug/`、`backups/`、`shell-snapshots/`（暫存與快取）
- `history.jsonl`（host CLI 歷史）

**保留**：`settings.json`、`settings.local.json`、`plugins/`、`skills/`、`.credentials.json`、`statusline-command.sh`、`.claude.json`（這個其實在 `~/.claude.json`，不在 `~/.claude/`）

**替代方案：**

- *用 rsync `--exclude`*：image 要加裝 rsync，與 cp 相比沒有實質好處
- *不排除全部複製*：磁碟用量大、敏感資料無謂進容器
- *讓使用者自己決定*：透過環境變數 `PERCH_CLAUDE_EXCLUDE` 客製。先不做，預設清單足夠

**理由**：cp + rm 是最簡單的組合；image 不用加套件；排除清單寫死在 entrypoint，易讀易改。

### D3：`.claude.json` seed 用 jq，不用 node

**決策**：用 `jq` 在 entrypoint 內 seed onboarding flag；image 預裝 `jq`。

**替代方案：**

- *新增一個 node script*：與 `merge-settings.js` 一致風格，但 entrypoint 已用 sh 為主；`.claude.json` 是 root JSON，jq 一行夠用。
- *Go binary 啟動時自己 seed*：把 `.claude.json` 邏輯放進 perch 本身。違背 separation of concerns（perch 不該動 Claude Code 的設定檔）。

**理由**：jq 是常見工具、image 加裝成本低、邏輯只有「缺 X 就 set X」幾個 expression。

### D4：seed 條件——「缺欄位才補」，不覆寫使用者選擇

**決策**：seed 步驟逐欄位檢查，只在欄位缺失（不存在或 null）時補；存在的值（包含 false）一律保留。

**理由**：使用者可能刻意把 `hasAcceptedAllTerms` 設為 false 等待自己接受。seed 的目的是「讓 fresh 容器能跑」，不是強加 perch 的偏好。

## Risks / Trade-offs

- **磁碟占用**：cp 副本約 5–20MB（排除 sessions/projects/cache 後）。可接受。
- **host 端變更不即時**：host `~/.claude` 改了 credentials 或 settings 要重啟容器才同步。對 perch 長跑情境可接受。
- **claude-agent-acp 模式不受影響**：ACP 路徑透過 stdio 與 subprocess 互動，沒有 trust dialog / theme dialog 阻塞問題。Container compat 修改本身不影響 ACP；後續 `consolidate-acp-runtime` 會把 chat-API 與 IM 全面 ACP 化。
- **未來 Claude Code 升級**：若 Claude Code 新增 volatile dir，cp 副本本來就 RW 整個目錄，不會壞掉（與 tmpfs 方案相比優勢明顯）。若新增 onboarding flag 才會卡，要更新 D4 的 seed 清單。
- **entrypoint 失敗模式**：cp 失敗、jq 不存在、merge 失敗——entrypoint 都應 log warning 但繼續 exec perch；不要因為配置 helper 失敗而 block 啟動。
- **Breaking change for existing deploys**：mount 路徑由 `${HOME}/.claude:/home/perchuser/.claude:ro` 改為 `${HOME}/.claude:/etc/perch-claude-host:ro`。既有 deploy 的 docker-compose 必須升級。release note 要清楚標明。

## Migration Plan

不需要資料遷移——所有改動是新增容器啟動行為，既有 image 重 build 就生效：

1. **Phase 1 — Implementation**：改 `entrypoint.sh`、`Dockerfile`、`tests/docker-compose.local-test.yml`、新增 jq 套件
2. **Phase 2 — Validation**：跑 batch B QA cycle（MT12 / T55 / T56 / T19 / T27 / T33-forward），全 PASS
3. **Phase 3 — Cleanup**：刪除 `tests/test-claude-rw/`、`tests/test-perchuser/.claude.json` 兩個 fixture（這次 cycle 已先刪），`tests/test-data/` `tests/test-workspace/` 加進 `.gitignore`
4. **Phase 4 — Docs**：README 加「container 啟動會自動 seed onboarding flag、merge user-level hooks、要求 tmpfs mount 兩個 volatile dir」段落

## Open Questions

_All resolved._

- ~~**Q1**：fresh container 無 host claude staging mount 時的行為？~~ **已決**：`mkdir -p /home/perchuser/.claude`（空白），由 D4 seed 處理 `.claude.json`，使用者首次 `claude /login` 完成認證。
- ~~**Q2**：`PERCH_CLAUDE_EXCLUDE` 是否可配置？~~ **已決**：不做，預設排除清單夠用，有需求時再加。
