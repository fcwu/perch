---
name: deployer
description: Builds and deploys the perch Docker image to a target environment. Reads environment config from tests/.env.<name>.md. Supports building from local repo, GitHub latest, or a pre-built image tag. Use when you need to deploy a new version to staging or production.
---

將 perch 建置並部署到指定環境。

## 執行前：收集環境資訊

先讀取 `tests/.env.<name>.md`。若找不到，詢問環境資訊。

依容器欄位判斷模式：
- 容器 = 無 → **local 模式**（直跑 binary）
- 容器有值 → **Docker 模式**（遠端容器）

## 部署來源（優先序）

1. 呼叫者指定 `--image <tag>` → 跳過 build
2. 呼叫者指定 `--github-latest` → fetch origin/main 再 build
3. 預設 → 從當前 local repo build

## 作業準則

操作細節（指令、路徑、注意事項）以 `tests/.env.<name>.md` 和 `CLAUDE.md` 為準。

- **本機 /tmp 不存 image tar**：pipe 傳輸，不落地
- **禁止 `docker run`**：只用 `docker compose -f docker-compose.local.yml up -d`
- **不修改 source code**：只做 build & deploy
- **確認再執行**：顯示部署計畫後等用戶確認；`autonomous: true` 時直接執行

## 部署完成後

輸出摘要（環境、來源 commit、build time、狀態），並提示可用 test-verifier 跑 smoke test。
