# 多連線與 PTY 管理 測試案例

> 功能：multi-connection-pty
> 涵蓋範圍：Framebuffer replay、雙向輸入同步、多行 URL 偵測。
> 撰寫日期：2026-04-20

---

## T11 — 多連線 Framebuffer Replay

**層級**：E2E-browser

**操作方式**：可用 CDP 指令開多個 browser tab 模擬多連線（`node $CDP nav <targetA> <url>` 開啟分頁 A 後，再用 `node $CDP nav <targetB> <url>` 開啟分頁 B 並截圖比較）。

**Given** 使用者已在瀏覽器分頁 A 使用 terminal，畫面上有 Claude Code 的輸出內容
**When** 使用者在另一個分頁 B 開啟同一個 Perch URL
**Then** 分頁 B 立即顯示與分頁 A 相同的 terminal 畫面，無需等待或重新操作

---

## T14 — 多連線雙向輸入同步

**層級**：E2E-browser

**操作方式**：可用 CDP 指令開多個 browser tab 模擬多連線。在分頁 A 用 `node $CDP type <targetA> <text>` 輸入，再用 `node $CDP shot <targetB> /tmp/shot.png` 截圖分頁 B 確認輸出同步。

**Given** 使用者同時開啟分頁 A 和分頁 B，兩者都連上同一個 Perch terminal
**When** 使用者在分頁 A 的 terminal 輸入指令並送出
**Then** 分頁 B 即時看到相同的輸出，不需重新整理

**When** 使用者接著在分頁 B 的 terminal 輸入另一個指令並送出
**Then** 分頁 A 也即時看到相同的輸出；兩個分頁始終呈現一致的 terminal 狀態

---

## T25 — 多行 URL 偵測與點擊

**層級**：E2E-browser

**前置操作**：需先在 terminal 中產生一個超過單行寬度的長 URL 輸出。可在 Claude Code 提示符號後輸入以下指令觸發：
```
echo "https://example.com/very/long/path/that/exceeds/terminal/width/and/needs/to/wrap/onto/next/line?param=value&another=longvalue"
```
確認 terminal 畫面中實際出現折行的 URL 後，再進行點擊驗證。若 Claude 正在處理其他任務導致 terminal 繁忙，需等待 Claude 回到提示符號後再觸發。

**操作方式**：可用 CDP 截圖（`node $CDP shot <target> /tmp/shot.png`）確認 URL 底線顯示；點擊可用 `node $CDP click <target> <selector>` 觸發，再用 `node $CDP list` 確認新分頁是否開啟且 URL 正確。

**Given** terminal 畫面中出現一個長到需要折行的 URL
**When** 使用者將滑鼠移到該 URL 上（即使 URL 跨越兩行）
**Then** 整個 URL 帶底線，且顯示手指游標，提示可點擊

**When** 使用者點擊該 URL
**Then** 在新分頁開啟完整正確的 URL，不因換行而切斷
