## ADDED Requirements

### Requirement: Agent 在回應文字中嵌入圖片語法

Agent 應透過在回應文字中嵌入一個或多個 `[image: <路徑>]` token 來表示圖片輸出。Perch 應在將回應轉發給任何用戶端之前，對 `RunCompleted` 酬載進行後處理，提取這些 token。

#### Scenario: 單張圖片語法提取成功

- **WHEN** `RunCompleted` 文字包含 `[image: /tmp/screenshot.png]`
- **THEN** Perch 提取該 token、讀取 `/tmp/screenshot.png`、儲存至 `<workdir>/images/<conv-id>/<uuid>.png`、從顯示文字中刪除該 token，並產生圖片附件記錄 `{url: "/api/images/<conv-id>/<uuid>.png", caption: "screenshot.png"}`

#### Scenario: 多張圖片語法依序提取

- **WHEN** `RunCompleted` 文字包含兩個以上 `[image: ...]` token
- **THEN** 每個 token 獨立提取並儲存；顯示文字刪除所有 token；回應依出現順序攜帶圖片附件記錄列表

#### Scenario: base64 內嵌圖片提取成功

- **WHEN** `RunCompleted` 文字包含 `[image: data:image/png;base64,<data>]`
- **THEN** Perch 解碼 base64 資料、儲存為 `<workdir>/images/<conv-id>/` 下的檔案，並產生帶有 `/api/images/...` URL 的圖片附件記錄

#### Scenario: 檔案不存在時靜默略過

- **WHEN** `RunCompleted` 文字包含 `[image: /tmp/nonexistent.png]` 且該檔案不存在
- **THEN** Perch 記錄警告、從顯示文字中刪除該 token，且不為該 token 新增圖片附件記錄

#### Scenario: 檔案過大時靜默略過並附加說明

- **WHEN** 被參照的圖片檔案超過 8 MB
- **THEN** Perch 記錄警告、刪除 token，並在顯示文字中以 `(圖片過大，無法顯示)` 取代該 token

#### Scenario: 路徑穿越嘗試被拒絕

- **WHEN** `[image: ...]` token 中的路徑解析後超出 `/tmp` 與 `<workdir>` 範圍
- **THEN** Perch 記錄安全警告、刪除 token，且不讀取或儲存該檔案

### Requirement: 圖片檔案透過驗證 HTTP 端點提供存取

Perch 應提供 `GET /api/images/<conv-id>/<filename>` 以供存取已儲存的 Agent 圖片。該端點應套用與其他 `/api/*` 端點相同的 session cookie 驗證。

#### Scenario: 已驗證請求正常回傳圖片

- **WHEN** 持有效 session cookie 的用戶端發送 `GET /api/images/<conv-id>/<uuid>.png`
- **THEN** Server 回應圖片位元組、`Content-Type: image/png` 與 `Cache-Control: private, max-age=86400`

#### Scenario: 未驗證請求被拒絕

- **WHEN** 未持有效 session cookie 的用戶端發送 `GET /api/images/<conv-id>/<uuid>.png`
- **THEN** Server 回應 401 Unauthorized

#### Scenario: 不存在的圖片回傳 404

- **WHEN** 用戶端請求圖片 store 中不存在的檔案
- **THEN** Server 回應 404 Not Found

### Requirement: 圖片 store 隨 ACP session 清理

儲存於 `<workdir>/images/<conv-id>/` 的 Agent 圖片，應在 ACP session pool 淘汰對應條目時刪除；Perch 啟動時應刪除超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS` 天的舊目錄。

#### Scenario: Session 淘汰時刪除圖片目錄

- **WHEN** ACP session pool 淘汰 `(user, conv-id)` 條目
- **THEN** Perch 刪除 `<workdir>/images/<conv-id>/` 及其所有內容

#### Scenario: 啟動時孤兒掃描刪除過期目錄

- **WHEN** Perch 啟動時發現 `<workdir>/images/<conv-id>/` 的 mtime 超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS`
- **THEN** Perch 刪除該目錄
