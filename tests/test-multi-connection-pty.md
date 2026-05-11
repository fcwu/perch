# 多連線與 PTY 管理 測試案例

> 功能：multi-connection-pty
> 涵蓋範圍：Framebuffer replay、雙向輸入同步、多行 URL 偵測。
> 撰寫日期：2026-04-20

---

## T11 — 多連線 Framebuffer Replay

**層級**：E2E-browser

**Given** 使用者已在瀏覽器分頁 A 使用 terminal，畫面上有 Claude Code 的輸出內容
**When** 使用者在另一個分頁 B 開啟同一個 Perch URL
**Then** 分頁 B 立即顯示與分頁 A 相同的 terminal 畫面，無需等待或重新操作

---

## T14 — 多連線雙向輸入同步

**層級**：E2E-browser

**Given** 使用者同時開啟分頁 A 和分頁 B，兩者都連上同一個 Perch terminal
**When** 使用者在分頁 A 的 terminal 輸入指令並送出
**Then** 分頁 B 即時看到相同的輸出，不需重新整理

**When** 使用者接著在分頁 B 的 terminal 輸入另一個指令並送出
**Then** 分頁 A 也即時看到相同的輸出；兩個分頁始終呈現一致的 terminal 狀態

---

## T25 — 多行 URL 偵測與點擊

**層級**：E2E-browser

**前置操作**：在 terminal 中執行 `echo` 輸出一個超過單行寬度的長 URL，確認 terminal 畫面出現折行的 URL 後再進行點擊驗證。

**Given** terminal 畫面中出現一個長到需要折行的 URL
**When** 使用者將滑鼠移到該 URL 上（即使 URL 跨越兩行）
**Then** 整個 URL 帶底線，且顯示手指游標，提示可點擊

**When** 使用者點擊該 URL
**Then** 在新分頁開啟完整正確的 URL，不因換行而切斷
