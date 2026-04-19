## 為什麼

Perch 目前只有一種固定的運作模式：GitLab OAuth 是唯一的認證方式，`/chat` 是唯一的入口。這無法服務兩種截然不同的使用情境：（1）操作員為自己單獨運行 Perch（無 GitLab，直接終端機存取），以及（2）操作員透過 GitLab OAuth 與團隊共享 Perch。路由、認證方式與 UI 都需要依據所選模式進行調整。

## 變更內容

- 新增 `PERCH_MODE` 環境變數（預設 `single` | `multi`），明確選擇運作模式。
- **單使用者模式**：`/` 直接提供終端機 UI。認證方式可設定：`none`（無認證）、`password`（`PERCH_PASSWORD`）、`mtls` 或 `gitlab`（GitLab OAuth）。使用 `gitlab` 認證時，`GITLAB_ADMIN_IDS` 限制哪些 GitLab 帳號可以登入。
- **多使用者模式**：需要 `PERCH_MODE=multi` 且已設定 GitLab OAuth。`/` 對未認證的使用者顯示登入畫面。`GITLAB_ADMIN_IDS` 中的使用者會被路由到 `/admin`（終端機 UI + 管理面板）；其他已認證的 GitLab 使用者則路由到 `/chat`。管理員也可以直接瀏覽 `/chat`。
- 新增 `GET /auth/logout` endpoint（清除 session cookie，重導向至 `/`）。
- 新增 `GET /api/auth/status` endpoint（公開，以 JSON 返回當前認證狀態）。
- 在多使用者模式下，未認證時 `/chat` 和 `/admin` 提供 React SPA，SPA 渲染內嵌登入畫面——不進行伺服器端重導向至 GitLab。
- GitLab auth middleware 對未認證的 API 呼叫返回 HTTP 401 JSON（而非 HTTP 302）。

## 能力

### 新增能力

- `operating-mode`：單使用者 vs 多使用者模式選擇及其路由規則。
- `auth-providers`：單使用者模式的可設定認證方式（none、password、mTLS、GitLab）。
- `gitlab-multi-user`：GitLab-backed 多使用者路由——管理員 vs 一般使用者的區分、允許名單、登入/登出 UI。

### 修改能力

<!-- 未修改任何現有規格——現有的 admin-auth 規格是獨立的。 -->

## 影響範圍

- **`server.go`**：模式感知路由；註冊 `/auth/logout`、`/api/auth/status`；在多使用者模式下移除 HTML shell 路由上的 GitLab middleware。
- **`gitlab_auth.go`**：新增 `allowedIDs`、`adminIDs` map；新增 `handleLogout`、`handleAuthStatus`；將 middleware 改為返回 401 而非重導向。
- **`auth.go`** / 新增 `auth_single.go`：單使用者模式的密碼與 mTLS 認證處理器。
- **`frontend/src/`**：模式感知 App 入口——多使用者未認證狀態的登入畫面；登出按鈕；管理員 vs 聊天路由。
- **不變更資料庫結構。**
