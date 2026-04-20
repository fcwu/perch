# 容器化與部署 測試案例

> 功能：container-deploy
> 涵蓋範圍：Working Directory 掛載、~/.claude 憑證掛載、PUID/PGID 非 root 執行。
> 撰寫日期：2026-04-20

---

## T13 — Working Directory Mount（Docker）

**層級**：E2E-browser

**Given** Docker container 啟動時將主機的專案目錄掛載至 `/workspace`
**When** 使用者在 terminal 中查看 `/workspace` 的檔案列表
**Then** 看到與主機專案目錄相同的檔案，可正常存取

---

## T15 — 掛載 ~/.claude → Claude Code 已登入

**層級**：E2E-browser

**Given** 主機上已完成 Claude Code 登入，且 Docker container 啟動時掛載了 `~/.claude`
**When** 使用者在瀏覽器開啟 `http://localhost:8080`，等待 terminal 就緒
**Then** Claude Code 直接顯示歡迎訊息或提示符號，不出現任何登入提示或 OAuth 相關畫面

---

## T16 — 未掛載 ~/.claude → Claude Code 未登入

**層級**：E2E-browser

**Given** Docker container 啟動時未掛載 `~/.claude`
**When** 使用者在瀏覽器開啟 terminal
**Then** terminal 顯示登入提示，引導使用者完成 Claude Code 認證，不會自動進入就緒狀態

**反向驗證**：掛載 `-v ~/.claude:/home/perchuser/.claude` 後重建 container，應直接進入已登入狀態（如 T15）。

---

## T30 — 非 root 容器（PUID/PGID）

**層級**：E2E-browser

**Given** Docker container 以 `PUID` 和 `PGID` 設定為主機使用者的 UID/GID 啟動
**When** 使用者在 terminal 中讓 Claude 建立或修改 workspace 中的檔案
**Then**
- 新建的檔案擁有者為主機使用者，而非 root
- Claude 可正常執行工具操作，不出現「不能以 root 執行」的錯誤訊息
- container 內行程以非 root 身份執行

**反向驗證**：不帶 PUID/PGID 啟動，`id` 應顯示預設非 root uid（1000），workspace 檔案仍非 root 所有。
