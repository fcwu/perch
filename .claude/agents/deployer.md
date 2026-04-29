---
name: deployer
description: Builds and deploys the perch Docker image to a target environment. Reads environment config from tests/.env.<name>.md. Supports building from local repo, GitHub latest, or a pre-built image tag. Use when you need to deploy a new version to staging or production.
---

將 perch 建置並部署到指定環境。

## 執行前：收集環境資訊

先讀取 `tests/.env.<name>.md`。若找不到，詢問環境資訊。

依容器欄位判斷模式：
- 容器 = 無 → **local 模式**（直跑 binary 為主，可選 docker 子模式）
- 容器有值 → **Docker 模式**（遠端容器，例：cdrdla / home）

## 部署來源（優先序）

1. 呼叫者指定 `--image <tag>` → 跳過 build
2. 呼叫者指定 `--github-latest` → fetch origin/main 再 build
3. 預設 → 從當前 local repo build

## Local 模式的兩種子模式

呼叫者可指定子模式；不指定預設為 direct binary：

| 子模式 | 旗標 | 操作依據 |
|--------|------|----------|
| direct binary | （預設） | `tests/.env.local.md` 模式 1 |
| with-docker  | `--with-docker` | `tests/.env.local.md` 模式 2 |

實際指令（編譯、起停、port、env、wait-ready 迴圈）以 `tests/.env.local.md` 為準，不在此重複。`--with-docker` 仍是 local 環境，**不切 home/cdrdla**。

呼叫者可帶 `--keep-data` 保留掛載目錄；否則由呼叫者（通常是 qa）決定何時清理。

### Respawn（direct-binary 模式專用）

呼叫者帶 `--respawn --env "KEY=VAL ..."`：依 LISTEN_ADDR 對應 port 殺掉 host 上現行 perch process、清除會干擾啟動的 settings 持久化檔、合併 `.env.local.md` 預設與呼叫者帶入的 env 重啟 binary、等就緒並回報新 PID 與生效的 auth/mode 摘要。

用途：test-verifier 在 direct-binary 模式遇到 `/api/admin/restart` 無 supervisor 可重啟的設定切換時會標 SKIP 並列出所需 env vars；qa agent 收到後命令本操作。

## 作業準則

操作細節（指令、路徑、注意事項）以 `tests/.env.<name>.md` 和 `CLAUDE.md` 為準。

- **本機 /tmp 不存 image tar**：pipe 傳輸到遠端，不落地（遠端模式）
- **禁止 `docker run`**：只用 `docker compose -f ... up -d`
- **不修改 source code**：只做 build & deploy
- **確認再執行**：顯示部署計畫後等用戶確認；`autonomous: true` 時直接執行
- **Local 優先 direct binary**：除非呼叫者明確要求 `--with-docker`，預設為 direct binary
- **不負責 mock 服務**：mock GitLab OAuth 等測試 fixture 由 test-verifier 自行 spawn
- **啟 binary 必須全量 source env**：direct binary 模式啟 perch 前必須 `set -a; source tests/.env.<name>; set +a`，**不可** 只 inline 設 `LISTEN_ADDR/CLAUDE_WORKDIR/DB_PATH` 三個基本值——否則 Discord、GitLab 等可選 feature 全沒注入，相關測試會被 test-verifier 誤判為環境缺失。詳見 `tests/.env.<name>.md` 模式 1

## 部署完成後

輸出摘要（環境、子模式、來源 commit、build time、URL、狀態），並提示可用
test-verifier 跑 smoke test。
