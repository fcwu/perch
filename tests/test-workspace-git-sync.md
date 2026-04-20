# Workspace Git Sync 測試案例

> 功能：workspace-git-sync
> 涵蓋範圍：sync 啟停、HTTPS/SSH remote、token 注入、衝突偵測與通知、push 失敗通知、非 git 目錄。
> 相關 openspec：`auto-git-sync`。
> 撰寫日期：2026-04-20

---

## T44 — Workspace Git Sync：功能停用（預設行為）

**層級**：E2E-curl

**Given** workspace 是一個 git repo，但 Perch 未設定 `WORKSPACE_GIT_SYNC_ENABLED`
**When** Perch 啟動並運作
**Then** 不進行任何 git 同步，log 中沒有任何與 workspace sync 相關的訊息

---

## T45 — Workspace Git Sync：HTTPS remote 正常 pull + push

**層級**：E2E-curl

**Given** workspace 是一個 git repo，remote 為 HTTPS，且遠端有新的 commit；Perch 以有效的 git token 啟動並啟用同步
**When** 等待一個同步週期（約 15 秒）
**Then**
- log 顯示 token 注入（不含 token 值）和 credential 設定的成功訊息
- log 顯示同步開始與完成
- workspace 中出現遠端的新 commit（`git log` 可確認）

**反向驗證（不設 token）**：log 顯示「跳過 token 注入」的說明；pull/push 是否成功取決於系統 credential，但不會崩潰。

---

## T46 — Workspace Git Sync：rebase 衝突偵測與通知

**層級**：E2E-browser（含 Discord 整合）

**Given** workspace 本地和遠端修改了同一行（製造衝突），Perch 啟用同步並設定 Discord 通知 channel
**When** 同步週期觸發，嘗試 pull --rebase
**Then**
- 系統自動 abort rebase，不留下衝突狀態
- log 記錄衝突細節
- 指定的 Discord channel 收到 `⚠️ git sync conflict` 通知

**Debounce 驗證**：第二個同步週期觸發時，Discord 不重複發送同一則通知。

---

## T47 — Workspace Git Sync：push 失敗通知

**層級**：E2E-browser（含 Discord 整合）

**Given** 遠端已被 force push，導致 workspace 的 push 會被 reject；Perch 啟用同步並設定 Discord 通知 channel
**When** 同步週期觸發，嘗試 push
**Then**
- log 記錄 push 失敗的錯誤訊息（含 git 的錯誤說明）
- 指定的 Discord channel 收到 `⚠️ git sync: git push failed` 通知

---

## T48 — Workspace Git Sync：SSH remote 忽略 token

**層級**：E2E-curl

**Given** workspace 的 remote 為 SSH 格式，Perch 設有 `WORKSPACE_GIT_TOKEN`
**When** Perch 啟動並嘗試同步
**Then**
- log 顯示「SSH remote，token 被忽略」的說明
- 不嘗試注入 token 到 credential store
- 同步結果取決於系統 SSH 設定，但不 crash

---

## T49 — Workspace Git Sync：非 git 目錄不啟動

**層級**：E2E-curl

**Given** `/workspace` 存在但不是 git repo（無 `.git` 目錄），且 Perch 啟用了同步
**When** Perch 啟動
**Then**
- log 顯示「workspace 不是 git repo，跳過同步」的說明
- Perch 正常啟動，不 crash，不強制執行同步
