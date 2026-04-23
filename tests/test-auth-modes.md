# 認證模式 測試案例

> 功能：auth-modes
> 涵蓋範圍：Rate Limit、Password 認證、mTLS Bootstrap，含對應 unit test。
> 撰寫日期：2026-04-20

---

## T09 — Rate Limit

**層級**：E2E-curl

**前置操作**：需先切換到 password 模式（參考 `tests/.env.home2.md`「模式切換」→「Password 模式」），容器重啟後再執行測試，測試完畢後還原。密碼為 `testpass123`。

**Given** Perch 以 password 模式啟動（`AUTH_MODE=password`，`AUTH_PASSWORD=testpass123`）
**When** 使用者在短時間內對登入端點發送超過 5 次錯誤的密碼嘗試
**Then** 前 5 次收到「密碼錯誤」或「找不到」的正常錯誤回應；第 6 次起收到「請求過多」的限速回應（HTTP 429）

---

## T10 — 密碼模式

**層級**：E2E-curl

**前置操作**：需先切換到 password 模式（參考 `tests/.env.home2.md`「模式切換」→「Password 模式」），容器重啟後再執行測試，測試完畢後還原。密碼為 `testpass123`。

**Given** Perch 以 `AUTH_MODE=password` 及 `AUTH_PASSWORD=testpass123` 啟動
**When** 使用者以正確密碼登入
**Then** 登入成功，伺服器發放 session cookie，使用者可繼續存取頁面

**When** 使用者以錯誤密碼登入
**Then** 登入被拒絕，收到「未授權」錯誤（HTTP 401）

**反向驗證**：未帶 cookie 直接存取頁面，應收到「未授權」回應。

---

## T12 — mTLS Bootstrap 流程

**層級**：E2E-curl

**前置操作**：需先切換到 mTLS 模式（參考 `tests/.env.home2.md`「模式切換」→「mTLS 模式」，將 `AUTH_METHOD=mtls`），容器重啟後再執行測試，測試完畢後還原。注意：此模式有已知 Bug（generateClientP12 key mismatch），建議在 Bug 修復後執行。

**Given** Perch 以 `AUTH_MODE=mtls` 啟動
**When** 使用者首次造訪 `/bootstrap` 端點（不帶任何用戶端憑證）
**Then** 成功下載 `client.p12` 憑證檔案，可用於後續連線

**When** 使用者再次造訪 `/bootstrap`
**Then** 端點已失效，收到「已過期」回應（HTTP 410）

**When** 使用者不帶憑證造訪其他頁面
**Then** 自動被導向 `/bootstrap` 頁面，可完成首次設定

---

## T20 — Password 模式：所有端點受保護（unit test）

> **自動化**：`go test` → `TestAuthPasswordBlocksAllEndpoints`

**Given** Perch 以 password 模式啟動，使用者尚未登入
**When** 使用者嘗試存取任何受保護的頁面或功能（包含主頁、WebSocket、排程管理）
**Then** 收到「未授權」回應（HTTP 401），無法取得任何內容

---

## T21 — Password 模式：/login 與 /bootstrap 不需 session（unit test）

> **自動化**：`go test` → `TestAuthPasswordBypassEndpoints`

**Given** Perch 以 password 模式啟動，使用者尚未登入
**When** 使用者造訪登入頁或 bootstrap 端點
**Then** 可以正常到達這些頁面，不被擋下（否則使用者將永遠無法完成登入）

---

## T22 — Password 模式：Session Cookie 無 Secure Flag（unit test）

> **自動化**：`go test` → `TestAuthPasswordSessionCookieNotSecure`

**Given** Perch 以 password 模式（plain HTTP）啟動
**When** 使用者成功登入，伺服器發放 session cookie
**Then** cookie 不帶 `Secure` 屬性，瀏覽器可在 HTTP 連線下正常回送 cookie，使用者保持登入狀態

---

## T23 — mTLS 模式：無 Client Cert 自動跳轉 /bootstrap（unit test）

> **自動化**：`go test` → `TestAuthMTLSRedirectsWithoutClientCert`

**Given** Perch 以 mTLS 模式啟動，使用者尚未安裝用戶端憑證
**When** 使用者嘗試造訪任何頁面
**Then** 自動被導向 `/bootstrap` 頁面，可從那裡完成首次設定

---

## T24 — mTLS 模式：/bootstrap 不需 Client Cert（unit test）

> **自動化**：`go test` → `TestAuthMTLSBootstrapAccessibleWithoutClientCert`

**Given** Perch 以 mTLS 模式啟動，使用者沒有用戶端憑證
**When** 使用者造訪 `/bootstrap`
**Then** 可以正常到達並下載憑證，不被擋下（否則形成無法打破的雞生蛋困境）

---

## 已知 Bug

### Bug：mTLS generateClientP12 key mismatch（T12 相關）

`AUTH_MODE=mtls` 下，`/bootstrap` 產生的用戶端憑證與私鑰不匹配，導致 TLS 握手失敗：

```
x509: provided PrivateKey doesn't match parent's PublicKey
```

影響範圍：T12 整個流程無法完成。其他認證模式（none、password）不受影響。
