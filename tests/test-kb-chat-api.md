# KB Query Portal / Chat API 測試案例

> 功能：kb-chat-api
> 涵蓋範圍：GitLab OAuth 登入、Chat API、多使用者並行 session、Admin 功能、Rate Limit、Log 格式、Analytics。
> 相關 openspec：`kb-query-portal-phase1`、`kb-query-portal-phase2`、`kb-query-portal-phase3`。
> 撰寫日期：2026-04-20

---

## Unit / Integration（無需啟動 server）

### T57 — Per-User Rate Limit 429 回應（Phase 3）

**層級**：E2E-curl

**Given** Perch 設定每分鐘最多 2 次查詢，使用者已在短時間內送出 2 次
**When** 使用者嘗試送出第 3 次查詢
**Then** 收到「請求過多」的回應（HTTP 429），並得知多久後可再試

**自動化**：`go test -run TestUserRateLimiter ./...`

---

### T59 — Analytics API 回傳正確統計（Phase 3）

**層級**：E2E-curl

**Given** 系統中已有若干完成的查詢記錄（含工具呼叫事件）
**When** 管理員查詢指定時間範圍的使用統計
**Then** 回傳每位使用者的查詢次數與平均耗時（依查詢次數降冪排列），以及最常使用的工具排行（最多 10 個），和整體合計數據

**When** 時間範圍內無資料
**Then** 回傳空的使用者清單和工具排行，查詢總數為 0

**自動化**：`go test -run TestGetUsageStats ./...`

---

### T62 — Analytics API JOIN Query 不報 Ambiguous Column Error

**層級**：E2E-curl

**Given** 系統中有查詢記錄和工具呼叫記錄
**When** 管理員請求使用統計數據
**Then** 成功取得統計結果，不出現伺服器錯誤（HTTP 500）

**When** 時間範圍內無資料
**Then** 回傳空的統計結果，不出現錯誤

**自動化**：`go test -run TestGetUsageStats ./...`

---

## E2E-curl — 特定設定（LOG_FORMAT=json）

### T58 — JSON Log 格式驗證（Phase 3）

**層級**：E2E-curl

**Given** Perch 以 `LOG_FORMAT=json` 啟動
**When** 使用者送出一次查詢
**Then** 系統 log 中依序出現查詢開始、工具呼叫、查詢完成的 JSON 記錄，每筆含 time、level、msg、user_id、session_id 等欄位

**反向驗證**：不設 `LOG_FORMAT`（預設 text），log 輸出為純文字格式（`time=... level=INFO msg=...`）。

**自動化**：`go test -run TestLogger ./...`

---

## E2E-browser — 預設設定（含 Admin Token）

### T54 — Admin Login（Phase 2）

**層級**：E2E-browser

**Given** Perch 以有效的 `ADMIN_TOKEN` 啟動
**When** 管理員在 `/admin/login` 輸入正確的 token 並登入
**Then** 登入成功，管理員取得 admin session，可存取管理功能

**When** 管理員輸入錯誤的 token
**Then** 登入被拒絕

**反向驗證（未設 ADMIN_TOKEN）**：管理功能整體停用，存取管理頁面收到「服務無法使用」的提示。

**自動化**：`go test -run TestAdminLogin ./...`

---

### T52 — Chat UI：查詢送出與 markdown 串流

**層級**：E2E-browser

**Given** 使用者已登入並開啟 `/chat`
**When** 使用者在輸入框輸入問題並送出
**Then**
- 輸入框暫時停用，顯示「⟳ Thinking…」等待提示
- 回應以 markdown 格式逐步顯示在畫面上
- Tool calls 面板即時顯示工具名稱與執行狀態（spinner）
- 每個工具完成後，spinner 變為 ✓ 並顯示耗時
- 完成後輸入框恢復可用，可繼續輸入

**When** 使用者在上一個查詢尚未完成時再次送出查詢
**Then** 第二次查詢被拒絕，提示「上一個查詢仍在執行中」

---

### T55 — Admin 即時監控（Phase 2）

**層級**：E2E-browser

**Given** 管理員已登入 `/admin`，另一位使用者正在送出查詢
**When** 管理員查看 Live Sessions 面板
**Then**
- 新查詢開始時，面板出現新的 row，顯示使用者名稱、查詢摘要、耗時與目前執行的工具
- 當工具切換時，目前工具欄位即時更新，不需重整頁面
- 查詢結束後，該 row 從面板消失

---

### T56 — Admin 歷史搜尋（Phase 2）

**層級**：E2E-browser

**Given** 系統中已有若干完成的查詢記錄，管理員已登入
**When** 管理員開啟歷史記錄頁面，在搜尋欄輸入關鍵字
**Then** 列表即時過濾，只顯示符合的記錄

**When** 管理員點擊某一筆記錄
**Then** 展開詳情頁，可看到完整的查詢內容、回應、以及工具呼叫的時序

---

### T60 — GET /admin/login 應回傳 SPA HTML

**層級**：E2E-browser

**Given** Perch 已啟動
**When** 使用者在瀏覽器網址列直接輸入 `/admin/login` 並按 Enter
**Then** 頁面顯示「Admin Login」表單（含 token 輸入框和 Login 按鈕），不出現「method not allowed」或任何純文字錯誤

---

### T61 — Admin Tab 切換應為 Client-Side Routing

**層級**：E2E-browser

**Given** 管理員已登入 `/admin`
**When** 管理員點擊「History」tab、「Analytics」tab，再點回「Live」tab
**Then**
- 網址列隨之更新（如 `/admin/history`、`/admin/analytics`），但頁面不重新載入
- 切換過程中，瀏覽器只發出 API 資料請求，不發出整頁 HTML 請求
- 重新整理頁面時，仍顯示對應 tab 的內容

---

## E2E-browser — GitLab 模式

### T50 — GitLab OAuth：未登入自動導向登入

**層級**：E2E-browser

**前置條件**：需以 `AUTH_METHOD=gitlab`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`、`GITLAB_URL` 啟動本機 binary，並在瀏覽器對應的 URL 操作。

**Given** GitLab OAuth 已設定，使用者尚未登入（沒有 session cookie）
**When** 使用者在瀏覽器開啟 `/chat`
**Then** 頁面顯示 Chat UI 版面（左側 sidebar + 中央 chat 輸入區），sidebar 內嵌顯示「Login with GitLab」登入按鈕（不強制跳轉），使用者可點擊按鈕進入 GitLab 授權流程

**反向驗證（竄改 cookie）**：若使用者持有非法的 session cookie，存取 `/chat` 的 API 呼叫（`/api/chat`）收到「未授權」回應而非跳轉。

**反向驗證（state mismatch）**：OAuth callback 的 state 不一致時，使用者看到「請求無效」的錯誤，而非被允許登入。

---

### T51 — GitLab OAuth：完整登入流程

**層級**：E2E-browser（需連接 GitLab instance）

**前置條件**：需真實 GitLab instance 並完成 OAuth Application 設定（需 GitLab 回調可達），此為真實 E2E 測試，無法用 mock 替代。

**Given** 使用者尚未登入，GitLab OAuth Application 已建立
**When** 使用者開啟 `/chat`，被導向 GitLab 授權頁面，在 GitLab 頁面授權後返回
**Then**
- 使用者被導向 `/chat` 並可正常使用
- 瀏覽器中設有 `perch_session` cookie（HttpOnly、8 小時有效期）
- 無需再次登入即可繼續使用

---

### T53 — Chat UI：多使用者並行 Session 互不干擾

**層級**：E2E-browser（雙帳號）

**Given** 使用者 A 和使用者 B 各自以不同 GitLab 帳號登入
**When** 兩人同時各自送出查詢
**Then**
- A 的回應只出現在 A 的視窗，B 的回應只出現在 B 的視窗
- 任一使用者的查詢完成不影響另一使用者的 session 繼續執行

---

## 已知 Bug

### Bug：Admin Live Sessions 只顯示使用工具的 session（T55 相關）

對於不呼叫任何工具的簡單查詢（例如 "say hi"），session 在 Live Sessions 面板的出現與消失間距不到 1ms，前端來不及渲染。

**影響範圍**：只有 no-tool 查詢在 Live Sessions 看不到；需要讀檔/搜尋的真實 KB 查詢可正常顯示。

**建議驗證方式**：使用會觸發工具呼叫的查詢（如「列出 /workspace 的檔案」），觀察 Live Sessions 即時更新。

---

### Bug：Chat UI textarea 送出後未清空（T52 相關，低優先）

使用者按下 Enter 或 Send 送出查詢後，輸入框在某些情況下未立即清空，使用者需手動刪除才能輸入下一個問題。
