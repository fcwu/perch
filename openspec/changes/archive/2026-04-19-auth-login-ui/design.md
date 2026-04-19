## 背景

Perch 目前只支援 GitLab OAuth，且在 `server.go` 中無條件連線。沒有模式概念、沒有登出、沒有允許名單，且 middleware 執行硬性重導向而非返回 401。需要支援兩種截然不同的使用情境：

- **單使用者**：操作員為自己運行 Perch；希望在 `/` 直接存取終端機，認證摩擦最小甚至為零。
- **多使用者**：操作員與團隊共享 Perch；希望有 GitLab OAuth、依使用者路由（管理員 vs 聊天），以及允許名單。

## 目標 / 非目標

**目標：**
- `PERCH_MODE` 在啟動時選擇單使用者或多使用者行為。
- 單使用者支援四種認證方式：none、password、mTLS、GitLab。
- 多使用者需要 GitLab；將管理員路由到 `/admin`，其他人路由到 `/chat`。
- 管理員在多使用者模式下可以直接瀏覽 `/chat`。
- 未認證使用者在多使用者模式下顯示內嵌登入畫面（不進行伺服器端重導向）。
- 在兩種模式下（若已設定認證），登出均可運作。

**非目標：**
- 在運行時動態切換模式。
- 同時使用多個認證提供者。
- 管理員/非管理員以外的細粒度使用者權限。
- 跨伺服器重啟的持久化 session。

## 決策

### D1 — PERCH_MODE 是明確的切換開關，不自動偵測

**決策：** `PERCH_MODE=single`（預設）或 `PERCH_MODE=multi`。伺服器在啟動時讀取此值。若 `PERCH_MODE=multi` 但缺少 GitLab OAuth 環境變數，伺服器拒絕啟動並顯示清楚的錯誤訊息。

**理由：** 隱式偵測（例如「若已設定 GitLab 變數 → multi」）令人意外。明確的切換開關讓意圖清晰，並防止在 GitLab 僅用於單使用者認證時意外暴露多使用者功能。

### D2 — 單使用者認證透過 AUTH_METHOD 環境變數選擇

**決策：** `AUTH_METHOD` 環境變數選擇單使用者模式的認證方式：

| 值 | 行為 |
|---|---|
| `none`（預設）| 無認證——`/` 對所有人開放 |
| `password` | HTTP Basic 或表單登入；`PERCH_PASSWORD` 環境變數 |
| `mtls` | 需要客戶端憑證；TLS 設定已由 `tls.go` 處理 |
| `gitlab` | GitLab OAuth；需要 `GITLAB_*` 變數。`GITLAB_ADMIN_IDS` 限制允許的帳號 |

在單使用者模式下，`GITLAB_ADMIN_IDS` 作為允許名單——只有這些 ID 可以完成 OAuth。單使用者模式中沒有「一般」GitLab 使用者。

### D3 — 多使用者路由由 GITLAB_ADMIN_IDS 成員資格決定

**決策：** 在多使用者模式的 OAuth 後，伺服器檢查使用者的 GitLab ID 是否在 `GITLAB_ADMIN_IDS` 中。若是 → 重導向至 `/admin`；若否 → 重導向至 `/chat`。兩個路由都對瀏覽器可存取（HTML 無需認證提供）；API endpoint 執行認證。

`GITLAB_ALLOWED_IDS` 控制哪些非管理員使用者可以登入 `/chat`：
- **未設定或空白** → 不允許非管理員使用者（拒絕所有一般使用者）
- **`*`** → 允許任何已認證的 GitLab 使用者
- **逗號分隔的 ID** → 只允許這些特定 ID

`GITLAB_ADMIN_IDS` 中的使用者無論 `GITLAB_ALLOWED_IDS` 如何設定，始終被允許。

**管理員存取 /chat：** 在多使用者模式下，`/chat` API endpoint 檢查有效的 session（任何角色）。直接瀏覽 `/chat` 的管理員可以正常使用。

### D4 — HTML shell 路由無需認證 middleware 即可提供

**決策：** `/`、`/chat`、`/admin` 始終返回 `index.html`。認證執行僅在 API 層（`/api/chat`、`/api/admin/*` 等）。React SPA 在掛載時呼叫 `GET /api/auth/status` 以決定認證狀態並相應渲染。

**理由：** HTML 路由上的伺服器端重導向破壞了 SPA 模型，使得顯示內嵌登入畫面或錯誤訊息變得不可能。API 層是正確的執行邊界。

**`GET /api/auth/status` 回應：**
```json
{
  "authenticated": true,
  "username": "alice",
  "role": "admin",        // "admin" | "user" | ""
  "mode": "multi"         // "single" | "multi"
}
```

### D5 — gitlabAuth.middleware 返回 HTTP 401 JSON，而非 HTTP 302

**決策：** 對於缺失/無效 session，middleware 返回帶有 `{"error":"unauthorized"}` 的 HTTP 401。

**理由：** API 消費者不應跟隨重導向。前端明確透過 `/api/auth/status` 檢查登入狀態；它不依賴 middleware 重導向行為。

### D6 — 透過 GET /auth/logout 登出

**決策：** 公開的 `GET /auth/logout` 清除 session cookie（MaxAge=-1）並重導向至 `/`。冪等——對已認證和未認證的呼叫者均有效。

### D7 — 密碼認證使用 HTTP Basic，簡單直接

**決策：** `AUTH_METHOD=password` 使用 HTTP Basic Auth（瀏覽器原生，不需要登入頁面 HTML）。驗證 `PERCH_USERNAME`（預設：`admin`）和 `PERCH_PASSWORD`。首次成功認證後發放 session cookie，避免重複提示。

**考慮過的替代方案：** HTML 表單登入頁面。對於單使用者模式已拒絕——為通常只有一人使用的情境增加前端複雜性。如有需要，之後可以加入。

## 風險 / 取捨

- **`AUTH_METHOD=none` 對網路上任何人開放**：文件記載為「僅限受信任網路」。這是未設定 GitLab 時的現有行為，無變更。
- **靜態 `GITLAB_ADMIN_IDS` 需要重啟才能變更**：此變更可接受；動態設定可以後續加入。
- **mTLS 需要啟用 TLS**：若 `AUTH_METHOD=mtls` 但未設定 TLS，伺服器拒絕啟動。

## 遷移計畫

1. 現有使用 GitLab OAuth 的部署：新增 `PERCH_MODE=multi` 以保持當前的多使用者行為。若不加，部署將以單使用者模式運行，使用 GitLab 作為認證方式。
2. 不變更結構，不需要資料遷移。
3. 回滾：還原二進位並移除 `PERCH_MODE` 環境變數。
