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

---

## 作業模式（Operating Mode）

### AL01 — 預設為 single-user 模式

**層級**：E2E-curl

**目的**：確認未設定 `PERCH_MODE` 時，伺服器以 single-user 模式啟動。

**步驟**：
```bash
AUTH_METHOD=none ./perch
curl -s http://localhost:8080/api/auth/status
```

**預期**：
- 伺服器正常啟動（無錯誤）
- `/api/auth/status` 回傳 `{"mode":"single", ...}`

---

### AL02 — 明確設定 PERCH_MODE=multi

**層級**：E2E-curl（mock GitLab URL 即可，不需真實 OAuth）

**目的**：確認 `PERCH_MODE=multi` 加上正確 GitLab 設定時，以 multi-user 模式啟動。

**步驟**：
```bash
PERCH_MODE=multi GITLAB_CLIENT_ID=xxx GITLAB_CLIENT_SECRET=yyy GITLAB_URL=https://gitlab.example.com ./perch
curl -s http://localhost:8080/api/auth/status
```

**預期**：`/api/auth/status` 回傳 `{"mode":"multi", ...}`

---

### AL03 — multi-user 模式缺少 GitLab 設定時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**目的**：確認 `PERCH_MODE=multi` 但缺少必要 GitLab 環境變數時，伺服器拒絕啟動。

**步驟**：
```bash
PERCH_MODE=multi ./perch
```

**預期**：伺服器拒絕啟動並輸出明確的設定錯誤訊息（包含缺少的環境變數名稱）。

---

### AL04 — PERCH_MODE 非法值時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
PERCH_MODE=unknown ./perch
```

**預期**：伺服器拒絕啟動並輸出描述性錯誤。

---

### AL05 — HTML shell routes 不強制驗證（multi-user 模式）

**層級**：E2E-curl（無需 GitLab，只需伺服器啟動）

**目的**：確認 `/`、`/chat`、`/admin` 在未登入狀態下回傳 HTTP 200 index.html。

**步驟**：
```bash
# 在 multi-user 模式啟動，未附帶 session cookie
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/chat
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/admin
```

**預期**：兩者均回傳 HTTP 200，response body 為 `index.html`（包含 `<!doctype html>`）。

---

### AL06 — GET /api/auth/status 為公開 endpoint，永遠回傳 200

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
curl -s http://localhost:8080/api/auth/status
```

**預期**：HTTP 200，JSON body 包含 `authenticated`、`username`、`role`、`mode` 四個欄位。

---

### AL07 — multi-user OAuth 後 admin 導向 /admin

**層級**：Integration（mock OAuth server）

**目的**：確認 `GITLAB_ADMIN_IDS` 中的使用者完成 OAuth 後被導向 `/admin`。

**Mock 方式**：用 `httptest.NewServer` 模擬 GitLab 的 `/oauth/token` endpoint，回傳指定 GitLab user ID（在 `GITLAB_ADMIN_IDS` 中）。

**步驟**：
1. 啟動 mock OAuth server，設定回傳 user ID = `777`。
2. 設定 `GITLAB_ADMIN_IDS=777`，`GITLAB_URL` 指向 mock server。
3. 觸發 OAuth callback（`GET /auth/callback?code=mock`）。

**預期**：OAuth callback 回應為 HTTP 302，`Location: /admin`。

---

### AL08 — multi-user OAuth 後一般使用者導向 /chat

**層級**：Integration（mock OAuth server）

**Mock 方式**：mock server 回傳不在 `GITLAB_ADMIN_IDS` 中的 user ID，設定 `GITLAB_ALLOWED_IDS=*`。

**步驟**：觸發 OAuth callback，mock 回傳 user ID = `999`（非 admin）。

**預期**：HTTP 302，`Location: /chat`。

---

### AL09 — Admin 可直接存取 /chat（multi-user 模式）

**層級**：Integration（mock OAuth server 取得 session cookie 後，curl 測試 API）

**步驟**：
1. 以 mock OAuth 取得 admin session cookie。
2. `curl -b cookies.txt -X POST http://localhost:8080/api/chat -d '{"query":"hi"}'`

**預期**：HTTP 200（非 403/401）。

---

## 驗證方法（Auth Providers）

### AL10 — AUTH_METHOD=none 不驗證

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
AUTH_METHOD=none ./perch
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/
```

**預期**：HTTP 200，無需任何憑證。

---

### AL11 — AUTH_METHOD=password SPA root 回傳 HTML，API endpoint 無憑證回傳 401

**層級**：E2E-curl（無需 GitLab）

**設計說明**：SPA 設計（D4）—— HTML routes（`/`）永遠回傳 `index.html`（HTTP 200），認證由前端 overlay 執行；受保護的 API/WS endpoint（`/ws`、`/input`、`/sessions`）無 session cookie 時回傳 HTTP 401。

**步驟**：
```bash
AUTH_METHOD=password PERCH_PASSWORD=secret ./perch

# HTML route：應回傳 200 + HTML（前端自行渲染登入畫面）
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/

# 受保護 API：無 session cookie 應回傳 401
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/sessions
```

**預期**：`GET /` → HTTP 200（HTML）；`GET /sessions` → HTTP 401。

---

### AL12 — AUTH_METHOD=password 正確憑證發放 session cookie

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
AUTH_METHOD=password PERCH_USERNAME=admin PERCH_PASSWORD=secret ./perch

# 第一次帶密碼
curl -c cookies.txt -u admin:secret -s -o /dev/null -w "%{http_code}" http://localhost:8080/

# 第二次用 cookie，不帶密碼
curl -b cookies.txt -s -o /dev/null -w "%{http_code}" http://localhost:8080/
```

**預期**：兩次均回傳 HTTP 200。

---

### AL13 — AUTH_METHOD=password 缺少 PERCH_PASSWORD 時拒絕啟動

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
AUTH_METHOD=password ./perch
```

**預期**：伺服器拒絕啟動並輸出設定錯誤。

---

### AL14 — AUTH_METHOD=mtls 自動生成 self-signed 憑證並正常啟動

**層級**：E2E-curl（無需 GitLab）

**設計說明**：`AUTH_METHOD=mtls` 不需要預先設定 `TLS_CERT`/`TLS_KEY`。伺服器啟動時自動生成 CA、server cert 與 client p12，並透過 `/bootstrap` endpoint 提供 client cert 下載。

**步驟**：
```bash
AUTH_METHOD=mtls ./perch

# 下載 client cert p12（首次存取）
curl -k -o client.p12 https://localhost:8080/bootstrap

# 帶 client cert 存取
curl -k --cert-type P12 --cert client.p12:perch \
  -o /dev/null -w "%{http_code}" https://localhost:8080/
```

**預期**：伺服器正常啟動；`/bootstrap` 回傳 p12 檔案；帶 client cert 存取回傳 HTTP 200。

---

### AL15 — AUTH_METHOD=gitlab（single-user）未登入時顯示 GitLab 登入按鈕

**層級**：E2E-browser（無需真實 GitLab — 只驗證 UI 渲染，不需完成 OAuth）

**目的**：確認 SPA 在未登入狀態自行渲染登入畫面，而非伺服器端重導向。

**步驟**：
1. 以 `AUTH_METHOD=gitlab`、`GITLAB_URL=http://fake.example` 啟動。
2. 瀏覽器開啟 `/`，未登入。

**預期**：SPA 顯示含「Login with GitLab」按鈕的登入畫面。DevTools Network 顯示 `GET /` 為 HTTP 200（非 302）。

---

### AL16 — GITLAB_ADMIN_IDS 限制 single-user GitLab auth 可登入帳號

**層級**：Integration（mock OAuth server）

**目的**：確認不在 allowlist 的帳號被拒絕。

**Mock 方式**：mock server 回傳 user ID = `999`，`GITLAB_ADMIN_IDS=123456`。

**步驟**：觸發 OAuth callback。

**預期**：HTTP 403，不發放 session cookie。

---

## GitLab Multi-User 存取控制

> **注意**：AL17–AL21 的核心邏輯（ID 比對與重導向決策）可全部用 Integration 層級 mock OAuth server 測試，無需真實 GitLab。

### AL17 — GITLAB_ALLOWED_IDS 未設定時拒絕非 admin 使用者

**層級**：Unit（直接測試 allowlist 比對函式）或 Integration（mock OAuth）

**步驟**：
- Unit：呼叫 `isAllowed(userID, adminIDs, allowedIDs)` with `allowedIDs=[]`，預期回傳 false。
- Integration：mock 回傳非 admin user ID，不設 `GITLAB_ALLOWED_IDS`，觸發 callback。

**預期**：HTTP 403，重導向至 `/?error=access_denied`。

---

### AL18 — GITLAB_ALLOWED_IDS=* 允許任何已驗證的非 admin 使用者

**層級**：Unit 或 Integration（mock OAuth）

**步驟**：mock 回傳非 admin user ID，設 `GITLAB_ALLOWED_IDS=*`。

**預期**：使用者被允許並重導向至 `/chat`。

---

### AL19 — GITLAB_ALLOWED_IDS 指定 ID — 列表內使用者被允許

**層級**：Unit 或 Integration（mock OAuth）

**步驟**：mock 回傳 user ID = `111`，設 `GITLAB_ALLOWED_IDS=111,222`。

**預期**：使用者被允許並重導向至 `/chat`。

---

### AL20 — GITLAB_ALLOWED_IDS 指定 ID — 列表外使用者被拒絕

**層級**：Unit 或 Integration（mock OAuth）

**步驟**：mock 回傳 user ID = `999`，設 `GITLAB_ALLOWED_IDS=111,222`。

**預期**：HTTP 403，重導向至 `/?error=access_denied`。

---

### AL21 — Admin 無視 GITLAB_ALLOWED_IDS 限制

**層級**：Unit 或 Integration（mock OAuth）

**步驟**：mock 回傳 user ID = `777`，設 `GITLAB_ADMIN_IDS=777`，不設 `GITLAB_ALLOWED_IDS`。

**預期**：使用者被允許並重導向至 `/admin`。

---

### AL22 — 未驗證的 API 呼叫回傳 401 JSON（非 302）

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
curl -s -w "\n%{http_code}" -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"hello"}'
```

**預期**：HTTP 401，response body 為 `{"error":"unauthorized"}`，回應標頭無 `Location` 欄位。

---

### AL23 — GET /auth/logout 清除 cookie 並重導向至 /

**層級**：E2E-curl（無需 GitLab — 可用 password auth 取得 session）

**步驟**：
```bash
# 用 password auth 取得 session cookie
AUTH_METHOD=password PERCH_PASSWORD=secret ./perch
curl -c cookies.txt -u admin:secret -s http://localhost:8080/

# 執行 logout
curl -b cookies.txt -v http://localhost:8080/auth/logout
```

**預期**：
- `Set-Cookie` header 含 `Max-Age=0`（清除 cookie）
- HTTP 302，`Location: /`

---

### AL24 — 未登入時也可呼叫 /auth/logout（idempotent）

**層級**：E2E-curl（無需 GitLab）

**步驟**：
```bash
curl -v http://localhost:8080/auth/logout
```

**預期**：HTTP 302，`Location: /`，無錯誤。

---

## 前端 UI 行為

### AL25 — multi-user 未登入時顯示內嵌登入畫面

**層級**：E2E-browser（無需真實 GitLab）

**目的**：確認 SPA 自行渲染登入畫面，不依賴伺服器端重導向。

**步驟**：
1. `PERCH_MODE=multi`、`GITLAB_URL=http://fake.example` 啟動。
2. 瀏覽器開啟 `/chat`，未登入。

**預期**：
- 顯示置中的登入畫面，含「Login with GitLab」按鈕。
- DevTools Network：`GET /chat` 為 HTTP 200，無 302。

---

### AL26 — 登入頁面顯示 access_denied 錯誤訊息

**層級**：E2E-browser（無需 GitLab）

**步驟**：瀏覽器開啟 `/?error=access_denied`。

**預期**：登入畫面顯示「Access denied. Contact the administrator.」訊息。

---

### AL27 — 已登入時顯示登出按鈕

**層級**：E2E-browser（可用 password auth，無需 GitLab）

**步驟**：
1. 以 `AUTH_METHOD=password` 完成登入。
2. 觀察頁面 UI。

**預期**：頁面可見登出按鈕。

---

### AL28 — 點擊登出按鈕導向 /auth/logout

**層級**：E2E-browser（可用 password auth，無需 GitLab）

**步驟**：已登入，點擊登出按鈕。

**預期**：瀏覽器發送 `GET /auth/logout`，session 清除，導回 `/`。

---

### AL29 — Single-user 模式已驗證時顯示 terminal UI

**層級**：E2E-browser（無需 GitLab）

**步驟**：
1. `PERCH_MODE=single`、`AUTH_METHOD=none` 啟動。
2. 瀏覽器開啟 `/`。

**預期**：SPA 渲染 terminal（Claude Code）UI，而非登入畫面。

---

### AL30 — Multi-user admin 在 /admin 看到 admin UI

**層級**：E2E-browser（Integration mock OAuth 取得 session 後，瀏覽器帶 cookie 瀏覽）

**步驟**：
1. 以 mock OAuth 取得 admin session cookie。
2. 瀏覽 `/admin`。

**預期**：SPA 渲染 terminal UI 與管理面板。

---

### AL31 — Multi-user 一般使用者在 /chat 看到聊天 UI

**層級**：E2E-browser（Integration mock OAuth 取得 session 後，瀏覽器帶 cookie 瀏覽）

**步驟**：
1. 以 mock OAuth 取得 user session cookie（`GITLAB_ALLOWED_IDS=*`）。
2. 瀏覽 `/chat`。

**預期**：SPA 渲染聊天 UI。
