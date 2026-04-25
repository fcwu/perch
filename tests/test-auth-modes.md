# 認證模式 測試案例

> 功能：auth-modes
> 涵蓋範圍：Rate Limit、Password 認證、mTLS Bootstrap，含對應 unit test。
> 撰寫日期：2026-04-20

---

## T09 — Rate Limit

**層級**：E2E-curl

**前置操作**（setup）：透過 Runtime Settings API 切換到 password 模式並等待重啟完成。

```bash
# 1. 取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 2. 切換到 password 模式（密碼設為 testpass123）
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "password", "password": "testpass123"}}'

# 3. 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 4. 等待 server 回來（輪詢 GET / 直到 HTTP 200）
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Server is back"
```

**Given** Perch 以 password 模式啟動（`AUTH_METHOD=password`，`AUTH_PASSWORD=testpass123`）
**When** 使用者在短時間內對登入端點發送超過 5 次錯誤的密碼嘗試
**Then** 前 5 次收到「密碼錯誤」或「找不到」的正常錯誤回應；第 6 次起收到「請求過多」的限速回應（HTTP 429）

```bash
# 驗證 rate limit 觸發
for i in $(seq 1 6); do
  echo -n "Attempt $i: "
  curl -s -o /dev/null -w "%{http_code}" -X POST $PERCH_URL/login \
    -H "Content-Type: application/json" \
    -d '{"password": "wrongpassword"}'
  echo
done
# 預期：前 5 次為 401，第 6 次起為 429
```

**後置操作**（teardown）：還原回 none 模式。

```bash
# 重新取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 切回 none 模式
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "none"}}'

# 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 等待回來
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Restored to none mode"
```

---

## T10 — 密碼模式

**層級**：E2E-curl

**前置操作**（setup）：透過 Runtime Settings API 切換到 password 模式並等待重啟完成。

```bash
# 1. 取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 2. 切換到 password 模式（密碼設為 testpass123）
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "password", "password": "testpass123"}}'

# 3. 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 4. 等待 server 回來
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Server is back"
```

**Given** Perch 以 `AUTH_METHOD=password` 及 `AUTH_PASSWORD=testpass123` 啟動
**When** 使用者以正確密碼登入
**Then** 登入成功，伺服器發放 session cookie，使用者可繼續存取頁面

```bash
# 以正確密碼登入，確認拿到 session cookie
SESSION_COOKIE=$(curl -s -c - -X POST $PERCH_URL/login \
  -H "Content-Type: application/json" \
  -d '{"password": "testpass123"}' | grep session_token | awk '{print $NF}')
echo "Session cookie: $SESSION_COOKIE"

# 用 session cookie 存取主頁，應收到 HTTP 200
curl -s -o /dev/null -w "%{http_code}" \
  -H "Cookie: session_token=$SESSION_COOKIE" \
  $PERCH_URL/
```

**When** 使用者以錯誤密碼登入
**Then** 登入被拒絕，收到「未授權」錯誤（HTTP 401）

```bash
curl -s -o /dev/null -w "%{http_code}" -X POST $PERCH_URL/login \
  -H "Content-Type: application/json" \
  -d '{"password": "wrongpassword"}'
# 預期：401
```

**反向驗證**：未帶 cookie 直接存取頁面，應收到「未授權」回應。

```bash
curl -s -o /dev/null -w "%{http_code}" $PERCH_URL/
# 預期：401
```

**後置操作**（teardown）：還原回 none 模式。

```bash
# 重新取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 切回 none 模式
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "none"}}'

# 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 等待回來
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Restored to none mode"
```

---

## T12 — mTLS Bootstrap 流程

**層級**：E2E-curl

> **注意**：此模式有已知 Bug（generateClientP12 key mismatch），建議在 Bug 修復後執行。

**前置操作**（setup）：透過 Runtime Settings API 切換到 mTLS 模式並等待重啟完成。

```bash
# 1. 取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 2. 切換到 mTLS 模式
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "mtls"}}'

# 3. 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 4. 等待 server 回來
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Server is back"
```

**Given** Perch 以 `AUTH_METHOD=mtls` 啟動
**When** 使用者首次造訪 `/bootstrap` 端點（不帶任何用戶端憑證）
**Then** 成功下載 `client.p12` 憑證檔案，可用於後續連線

```bash
curl -s -o client.p12 -w "%{http_code}" $PERCH_URL/bootstrap
# 預期：200，且 client.p12 非空
```

**When** 使用者再次造訪 `/bootstrap`
**Then** 端點已失效，收到「已過期」回應（HTTP 410）

```bash
curl -s -o /dev/null -w "%{http_code}" $PERCH_URL/bootstrap
# 預期：410
```

**When** 使用者不帶憑證造訪其他頁面
**Then** 自動被導向 `/bootstrap` 頁面，可完成首次設定

```bash
curl -s -o /dev/null -w "%{http_code}" $PERCH_URL/
# 預期：302，Location 指向 /bootstrap
```

**後置操作**（teardown）：還原回 none 模式。

```bash
# 重新取得 Admin Cookie
ADMIN_COOKIE=$(curl -s -c - -X POST $PERCH_URL/admin/login \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ADMIN_TOKEN\"}" | grep perch_admin | awk '{print $NF}')

# 切回 none 模式
curl -s -X PATCH $PERCH_URL/api/settings \
  -H "Content-Type: application/json" \
  -H "Cookie: perch_admin=$ADMIN_COOKIE" \
  -d '{"auth": {"method": "none"}}'

# 觸發重啟
curl -s -X POST $PERCH_URL/api/admin/restart \
  -H "Cookie: perch_admin=$ADMIN_COOKIE"

# 等待回來
until curl -sf $PERCH_URL/ > /dev/null 2>&1; do sleep 1; done
echo "Restored to none mode"
```

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
