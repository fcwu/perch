# 認證模式 測試案例

> 功能：auth-modes
> 涵蓋範圍：Rate Limit、Password 認證，含對應 unit test。
> 撰寫日期：2026-04-20

> **執行順序說明**：測試案例依 server 設定分組，減少 restart 次數。標注「共用一次 restart」的區段，前置操作只做一次，後置操作在整個區段結束後才執行。

---

## Unit / Integration（無需啟動 server）

## T20 — Password 模式：所有端點受保護（unit test）

> **自動化**：`go test` → `TestAuthPasswordBlocksAllEndpoints`

**Given** Perch 以 password 模式啟動，使用者尚未登入
**When** 使用者嘗試存取任何受保護的頁面或功能（包含主頁、WebSocket、排程管理）
**Then** 收到「未授權」回應（HTTP 401），無法取得任何內容

---

## T21 — Password 模式：/login 不需 session（unit test）

> **自動化**：`go test` → `TestAuthPasswordBypassEndpoints`

**Given** Perch 以 password 模式啟動，使用者尚未登入
**When** 使用者造訪登入頁
**Then** 可以正常到達，不被擋下（否則使用者將永遠無法完成登入）

---

## T22 — Password 模式：Session Cookie 無 Secure Flag（unit test）

> **自動化**：`go test` → `TestAuthPasswordSessionCookieNotSecure`

**Given** Perch 以 password 模式（plain HTTP）啟動
**When** 使用者成功登入，伺服器發放 session cookie
**Then** cookie 不帶 `Secure` 屬性，瀏覽器可在 HTTP 連線下正常回送 cookie，使用者保持登入狀態

---

## E2E-curl — Password 模式（共用一次 restart）

> 本區段所有測試共用一次 server 切換：區段開始時透過 `PATCH /api/settings` 切為 `auth.method=password`、`auth.password=testpass123` 並重啟；區段結束後切回 `auth.method=none` 並重啟。

### T10 — 密碼模式

**層級**：E2E-curl

**前置操作**：透過 `PATCH /api/settings` 將 `auth.method` 切為 `password`、`auth.password` 設為 `testpass123`，再 `POST /api/management/restart` 重啟並等待 server 回來。密碼為 `testpass123`。

**Given** Perch 以 `AUTH_METHOD=password` 及 `AUTH_PASSWORD=testpass123` 啟動
**When** 使用者以正確密碼登入
**Then** 登入成功，伺服器發放 session cookie，使用者可繼續存取頁面

**When** 使用者以錯誤密碼登入
**Then** 登入被拒絕，收到「未授權」錯誤（HTTP 401）

**反向驗證**：未帶 cookie 直接存取頁面，應收到「未授權」回應。

---

### T09 — Rate Limit

**層級**：E2E-curl

**後置操作**：`PATCH /api/settings` 將 `auth.method` 切回 `none`，再重啟並等待 server 回來。

**Given** Perch 以 password 模式啟動（`AUTH_METHOD=password`，`AUTH_PASSWORD=testpass123`）
**When** 使用者在短時間內對登入端點發送超過 5 次錯誤的密碼嘗試
**Then** 前 5 次收到「密碼錯誤」或「找不到」的正常錯誤回應；第 6 次起收到「請求過多」的限速回應（HTTP 429）
