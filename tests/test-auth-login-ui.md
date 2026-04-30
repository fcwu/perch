# Auth & Login UI 測試案例

> 功能：auth-login-ui
> 規格來源：`openspec/changes/auth-login-ui/specs/`
> 撰寫日期：2026-04-19

---

## 測試層級說明

| 層級 | 說明 | GitLab 相依 |
|------|------|-------------|
| **Unit** | Go unit test，mock `GitLabAuthProvider` interface，無需啟動伺服器 | 無 |
| **Integration** | `httptest` + mock OAuth server（`httptest.NewServer` 模擬 `/oauth/token`） | 無（mock） |
| **E2E-curl** | 啟動真實 perch binary，用 curl 驗證 HTTP 行為 | 無（不需 GitLab） |
| **E2E-browser** | 啟動真實 perch binary，瀏覽器手動操作 | 視情況 |
| **E2E-gitlab** | 需要連接真實 GitLab 實例完成 OAuth 流程 | **是** |

> **減少 GitLab 相依的原則**：OAuth callback 邏輯（ID 比對、重導向決策）可以用 Integration test 層級的 mock OAuth server 完整覆蓋，只有「真實瀏覽器走 GitLab 登入 UI」這一步無法取代。

> **執行順序說明**：測試案例依 server 設定分組，減少 restart 次數。標注「共用一次 restart」的區段，前置操作只做一次，後置操作在整個區段結束後才執行。

---

## Unit / Integration（無需啟動 server）

### AL07 — multi-user OAuth 後 admin 導向 /admin

**層級**：Integration（mock OAuth server）

**Given** 使用者的 GitLab user ID 在 `GITLAB_ADMIN_IDS` 清單中
**When** 使用者完成 GitLab OAuth 授權流程
**Then** 使用者被導向 `/admin` 頁面

---

### AL08 — multi-user OAuth 後一般使用者導向 /chat

**層級**：Unit/Integration（Go test with httptest mock）

**Go Test**：`TestGitLabAuthCallbackMultiModeAllowAllRedirectsToChat`（`gitlab_auth_test.go`）

**Given** 使用者的 GitLab user ID 不在 admin 清單中，且 `GITLAB_ALLOWED_IDS=*`
**When** 使用者完成 GitLab OAuth 授權流程
**Then** 使用者被導向 `/chat` 頁面

---

### AL09 — Admin 可直接存取 /chat（multi-user 模式）

**層級**：Integration（mock OAuth server 取得 session cookie 後驗證）

**Given** Admin 使用者已完成登入，持有有效的 session cookie
**When** Admin 向 `/api/chat` 送出查詢
**Then** 查詢被接受（非 403 / 401），Admin 可正常使用 chat 功能

---

### AL16 — GITLAB_ADMIN_IDS 限制 single-user GitLab auth 可登入帳號

**層級**：Integration（mock OAuth server）

**Go Test**：相關測試於 `gitlab_auth_test.go`

**Given** `GITLAB_ADMIN_IDS` 設定為特定的 user ID 清單，使用者的 ID 不在清單中
**When** 使用者完成 OAuth 授權
**Then** 登入被拒絕，使用者不取得 session cookie

---

### AL17 — GITLAB_ALLOWED_IDS 未設定時拒絕非 admin 使用者

**層級**：Unit 或 Integration（mock OAuth）

**Given** `GITLAB_ALLOWED_IDS` 未設定，使用者不是 admin
**When** 使用者嘗試完成 OAuth 登入
**Then** 登入被拒絕，使用者看到存取被拒的頁面

---

### AL18 — GITLAB_ALLOWED_IDS=* 允許任何已驗證的非 admin 使用者

**層級**：Unit/Integration（Go test with httptest mock）

**Go Test**：`TestGitLabAuthCallbackMultiModeAllowAllRedirectsToChat`（`gitlab_auth_test.go`）

**Given** `GITLAB_ALLOWED_IDS=*`，使用者不是 admin
**When** 使用者完成 OAuth 授權
**Then** 使用者被允許登入，並導向 `/chat` 頁面

---

### AL19 — GITLAB_ALLOWED_IDS 指定 ID — 列表內使用者被允許

**層級**：Unit/Integration（Go test with httptest mock）

**Go Test**：`TestGitLabAuthCallbackMultiModeAllowedListPermitted`（`gitlab_auth_test.go`）

**Given** `GITLAB_ALLOWED_IDS=55`，使用者的 ID 為 55
**When** 使用者完成 OAuth 授權
**Then** 使用者被允許登入，並導向 `/chat` 頁面

---

### AL20 — GITLAB_ALLOWED_IDS 指定 ID — 列表外使用者被拒絕

**層級**：Unit 或 Integration（mock OAuth）

**Given** `GITLAB_ALLOWED_IDS=111,222`，使用者的 ID 為 999（不在清單中）
**When** 使用者嘗試完成 OAuth 登入
**Then** 登入被拒絕，使用者看到存取被拒的頁面

---

### AL21 — Admin 無視 GITLAB_ALLOWED_IDS 限制

**層級**：Unit/Integration（Go test with httptest mock）

**Go Test**：`TestGitLabAuthCallbackAdminIgnoresAllowedIDs`（`gitlab_auth_test.go`）

**Given** 使用者的 ID 在 `GITLAB_ADMIN_IDS` 中，但不在 `GITLAB_ALLOWED_IDS` 中
**When** 使用者完成 OAuth 授權
**Then** 使用者被允許登入，並導向 `/admin` 頁面（admin 身份無視 allowed list 限制）

---

## E2E-curl — 預設設定（AUTH_METHOD=none）

### AL01 — 預設為 single-user 模式

**層級**：E2E-curl

**Given** 啟動 Perch 時未設定 `PERCH_MODE`
**When** Perch 啟動並開始服務
**Then** 伺服器以 single-user 模式運作，不出現任何錯誤

---

### AL03 — multi-user 模式缺少 GitLab 設定時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**Given** `PERCH_MODE=multi` 但未提供必要的 GitLab 環境變數
**When** 嘗試啟動 Perch
**Then** 伺服器拒絕啟動，並輸出包含缺少設定項目名稱的明確錯誤訊息

---

### AL04 — PERCH_MODE 非法值時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**Given** `PERCH_MODE` 設為未知的值（例如 `unknown`）
**When** 嘗試啟動 Perch
**Then** 伺服器拒絕啟動，並輸出描述性的設定錯誤

---

### AL06 — 任何人都可查詢認證狀態

**層級**：E2E-curl（無需 GitLab）

**Given** Perch 以任何模式啟動
**When** 使用者（含未登入者）查詢目前的認證狀態
**Then** 成功取得包含 `authenticated`、`username`、`role`、`mode` 欄位的回應

---

### AL13 — AUTH_METHOD=password 缺少 PERCH_PASSWORD 時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**Given** `AUTH_METHOD=password` 但未設定 `PERCH_PASSWORD`
**When** 嘗試啟動 Perch
**Then** 伺服器拒絕啟動，並輸出設定錯誤

---

### AL24 — 未登入時也可執行登出（idempotent）

**層級**：E2E-curl（無需 GitLab）

**Given** 使用者尚未登入（沒有 session cookie）
**When** 使用者觸發登出操作
**Then** 操作成功完成，被導向 `/chat`，不出現任何錯誤

---

## E2E-curl — mTLS 模式

### AL14 — AUTH_METHOD=mtls 自動生成 self-signed 憑證並正常啟動

**層級**：E2E-curl（無需 GitLab）

**Given** Perch 以 `AUTH_METHOD=mtls` 啟動（不預先設定 TLS 憑證）
**When** 伺服器啟動，使用者造訪 `/bootstrap` 頁面
**Then** 可下載 `client.p12` 憑證檔案；持憑證連線後可正常存取

---

## E2E-curl — Multi-user 模式

### AL02 — 明確設定 PERCH_MODE=multi

**層級**：E2E-curl（mock GitLab URL 即可，不需真實 OAuth）

**前置條件**：需以 `PERCH_MODE=multi` 及正確的 GitLab 設定（`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`、`GITLAB_URL`）啟動本機 binary（`go build` 後執行），mock GitLab URL 即可，不需真實 OAuth。

**Given** Perch 以 `PERCH_MODE=multi` 及正確的 GitLab 設定啟動
**When** Perch 啟動並開始服務
**Then** 伺服器以 multi-user 模式運作

---

## E2E-curl — Password 模式（共用一次 restart）

> 本區段所有測試共用一次 server 切換：區段開始時透過 `PATCH /api/settings` 切為 `auth.method=password`、`auth.password=testpass123` 並重啟；區段結束後切回 `auth.method=none` 並重啟。

### AL12 — AUTH_METHOD=password 正確憑證發放 session cookie

**層級**：E2E-curl（無需 GitLab）

**前置操作**：透過 `PATCH /api/settings` 將 `auth.method` 切為 `password`、`auth.password` 設為 `testpass123`，再 `POST /api/management/restart` 重啟並等待 server 回來。**後置操作**：區段結束後統一切回。

**Given** Perch 以 `AUTH_METHOD=password` 啟動
**When** 使用者以正確的帳號密碼登入
**Then** 伺服器發放 session cookie；之後使用者憑 cookie 即可繼續存取，不需再次輸入密碼

---

### AL22 — 未驗證的 API 呼叫回傳 401（非跳轉）

**層級**：E2E-curl（無需 GitLab）

**Given** 使用者未登入
**When** 使用者直接呼叫受保護的 API（如 `/api/chat`）
**Then** 收到「未授權」的 JSON 錯誤回應，不被跳轉到登入頁

---

### AL32 — password 模式下，GitLab env vars 存在仍可正常登入與使用 chat

**層級**：E2E-curl

**Given** Perch 以 `AUTH_METHOD=password` 啟動，且環境中同時存在 `GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`、`GITLAB_URL`
**When** 使用者以正確密碼呼叫 `POST /login`
**Then** 收到 HTTP 204，取得 `session` cookie

**When** 使用者持該 session cookie 呼叫 `POST /api/chat`
**Then** 收到 HTTP 200（或 409，若 session 已存在），**不**收到 401 或 503

```bash
BASE="http://localhost:8080"
curl -si -c /tmp/s.txt -X POST "${BASE}/login" \
  -H "Content-Type: application/json" -d '{"password":"changeme"}'
# 預期: HTTP 204, Set-Cookie: session=...

curl -si -b /tmp/s.txt -X POST "${BASE}/api/chat" \
  -H "Content-Type: application/json" -d '{"query":"hello"}'
# 預期: HTTP 200 或 409（非 401、非 503）
```

---

### AL33 — password 模式下，/auth/gitlab 不可用（404）

**層級**：E2E-curl

**後置操作**：`PATCH /api/settings` 將 `auth.method` 切回 `none`，再重啟並等待 server 回來。

**Given** Perch 以 `AUTH_METHOD=password` 啟動，且環境中存在 GitLab env vars
**When** 使用者（已登入或未登入）造訪 `/auth/gitlab`
**Then** 收到 HTTP 404，**不**被導向 GitLab OAuth 授權頁面

```bash
curl -si http://localhost:8080/auth/gitlab
# 預期: HTTP 404（非 302 redirect 到 GitLab）
```

---

## E2E-browser — 預設設定（AUTH_METHOD=none）

### AL10 — AUTH_METHOD=none 不驗證

**層級**：E2E-browser（無需 GitLab）

**Given** Perch 以 `AUTH_METHOD=none` 啟動
**When** 使用者直接開啟頁面，不提供任何憑證
**Then** 頁面正常載入，terminal 可使用，無任何驗證提示

---

### AL29 — Single-user 模式已驗證時重導向至 Chat UI

**層級**：E2E-browser（無需 GitLab）

**Given** Perch 以 `PERCH_MODE=single`、`AUTH_METHOD=none` 啟動
**When** 使用者開啟 `/`
**Then** 瀏覽器收到 302 重導向至 `/chat`；`/chat` 頁面直接顯示 Chat UI（sidebar + chat 輸入區），不出現任何登入畫面

---

## E2E-browser — Password 模式（共用一次 restart）

> 本區段所有測試共用一次 server 切換：區段開始時透過 `PATCH /api/settings` 切為 `auth.method=password`、`auth.password=testpass123` 並重啟；區段結束後切回 `auth.method=none` 並重啟。

### AL11 — AUTH_METHOD=password SPA root 回傳 HTML，前端顯示登入畫面

**層級**：E2E-browser（無需 GitLab）

**前置操作**：透過 `PATCH /api/settings` 將 `auth.method` 切為 `password`、`auth.password` 設為 `testpass123`，再 `POST /api/management/restart` 重啟並等待 server 回來。**後置操作**：區段結束後統一切回。

**Given** Perch 以 `AUTH_METHOD=password` 啟動
**When** 未登入的使用者開啟 `/`
**Then** 頁面正常載入，前端顯示登入畫面（伺服器回傳 HTML，非錯誤頁面）

**When** 未登入的使用者嘗試直接存取受保護的 API（如 `/sessions`）
**Then** 收到「未授權」的拒絕回應

---

### AL27 — 已登入時顯示登出按鈕

**層級**：E2E-browser（可用 password auth，無需 GitLab）

**Given** 使用者已以 password 模式完成登入
**When** 使用者查看頁面
**Then** 頁面上可見登出按鈕

---

### AL28 — 點擊登出按鈕清除 session 並返回首頁

**層級**：E2E-browser（可用 password auth，無需 GitLab）

**Given** 使用者已以 password 模式登入並看到登出按鈕
**When** 使用者點擊登出按鈕
**Then** session 被清除，使用者被導回 `/chat`（顯示登入畫面），登出按鈕消失

---

### AL23 — 登出後 session 清除並返回首頁

**層級**：E2E-browser（可用 password auth，無需 GitLab）

**後置操作**：`PATCH /api/settings` 將 `auth.method` 切回 `none`，再重啟並等待 server 回來。

**Given** 使用者已以 password 模式登入並持有 session cookie
**When** 使用者執行登出
**Then** session cookie 被清除，使用者被導向 `/chat`（或登入畫面），需重新登入才能存取

---

## E2E-browser — Multi-user 模式

> 本區段所有測試共用同一個 `PERCH_MODE=multi` + mock GitLab URL 啟動的 server。

### AL05 — 頁面在 multi-user 模式下不強制驗證即可載入

**層級**：E2E-browser（無需 GitLab）

**前置條件**：需以 `PERCH_MODE=multi` 啟動本機 binary，無需真實 GitLab，mock URL 即可。

**Given** Perch 以 multi-user 模式啟動，使用者尚未登入
**When** 使用者在瀏覽器開啟 `/chat` 或 `/admin`
**Then** 頁面正常載入（顯示登入畫面），不出現錯誤頁面或被完全擋下

---

### AL25 — multi-user 未登入時顯示內嵌登入畫面

**層級**：E2E-browser（無需真實 GitLab）

**前置條件**：需以 `PERCH_MODE=multi`、mock GitLab URL 啟動本機 binary，無需真實 GitLab OAuth。

**Given** Perch 以 multi-user 模式啟動，使用者尚未登入
**When** 使用者在瀏覽器開啟 `/chat`
**Then** 頁面顯示置中的登入畫面，含「Login with GitLab」按鈕；瀏覽器未被強制跳轉（頁面以正常載入的方式呈現）

---

### AL30 — Multi-user admin 在 /admin 看到 admin UI

**層級**：E2E-browser（偽造 session cookie，無需 GitLab OAuth）

**前置條件**：需以 `PERCH_MODE=multi` 啟動本機 binary，並偽造一個 admin role 的 session cookie（無需真實 GitLab OAuth，只需讓 perch 接受偽造 session）。

**Given** Perch 以 multi-user 模式啟動，使用者持有 admin role 的 session cookie
**When** 使用者開啟 `/admin`
**Then** 頁面顯示 admin 管理介面，不出現「Login with GitLab」按鈕

---

### AL31 — Multi-user 一般使用者在 /chat 看到聊天 UI

**層級**：E2E-browser（偽造 session cookie，無需 GitLab OAuth）

**前置條件**：需以 `PERCH_MODE=multi`、`GITLAB_ALLOWED_IDS=*` 啟動本機 binary，並偽造一般 user role session cookie。

**Given** Perch 以 multi-user 模式啟動（`GITLAB_ALLOWED_IDS=*`），使用者持有一般 user role 的 session cookie
**When** 使用者開啟 `/chat`
**Then** 頁面顯示對話輸入介面，不出現「Login with GitLab」按鈕

---

## E2E-browser — GitLab Single 模式

### AL15 — AUTH_METHOD=gitlab（single-user）未登入時顯示 GitLab 登入按鈕

**層級**：E2E-browser（無需真實 GitLab）

**前置條件**：需以 `AUTH_METHOD=gitlab`、`GITLAB_CLIENT_ID`、`GITLAB_CLIENT_SECRET`、`GITLAB_URL` 啟動本機 binary（mock GitLab URL 即可，不需真實 OAuth，只驗證前端是否渲染登入按鈕）。

**Given** Perch 以 `AUTH_METHOD=gitlab` 啟動，使用者尚未登入
**When** 使用者開啟 `/`
**Then** 頁面顯示含「Login with GitLab」按鈕的登入畫面；瀏覽器未被強制跳轉，頁面以正常載入（非 302 redirect）的方式呈現
