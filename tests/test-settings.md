# Settings Panel 測試案例

> 功能：settings-panel
> 涵蓋範圍：GET /api/settings 的 workspace env var 修復驗證、PATCH /api/settings 持久化、UI 各分頁渲染、workspace 欄位從 env 正確顯示、container down 時的錯誤處理。
> 撰寫日期：2026-04-30

---

## 測試層級說明

| 層級 | 說明 |
|---|---|
| **Unit** | Go unit test，直接測試 buildEnvSeed / SettingsManager |
| **Integration** | `httptest` 搭配 mock server 驗證 HTTP handler |
| **E2E-curl** | 啟動真實 binary，curl 驗證 HTTP 行為 |
| **E2E-browser** | 瀏覽器操作驗證前端 Settings panel |

---

## Unit / Integration（無需啟動 server）

### SP01 — buildEnvSeed 將 Workspace env vars 放入 seed

**層級**：Unit

> **自動化**：`go test -run TestBuildEnvSeed_WorkspaceFields ./...`

**Given** 程序啟動前設定 `WORKSPACE_GIT_SYNC_ENABLED=true`、`WORKSPACE_GIT_SYNC_INTERVAL=300`、`WORKSPACE_GIT_TOKEN=mytoken`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=alerts`、`WORKSPACE_GIT_SYNC_SUBMODULES=true`
**When** 呼叫 `buildEnvSeed()`
**Then** 回傳的 `RuntimeSettings.Workspace` 不為 nil，且各欄位值分別對應所設定的環境變數（sync_enabled=true、sync_interval="300"、git_token="mytoken"、notify_channel="alerts"、sync_submodules=true）

**When** 五個 workspace env vars 均為空值時呼叫 `buildEnvSeed()`
**Then** 回傳的 `RuntimeSettings.Workspace` 為 nil（不設預設值，避免覆蓋 settings.json）

---

### SP02 — GetEffective 合併 env seed 與 settings.json override

**層級**：Unit

> **自動化**：`go test -run TestGetEffective_WorkspaceMerge ./...`

**Given** env seed 含 `workspace.sync_interval="300"`，而 settings.json 已存有 `workspace.sync_interval="600"`（JSON override）
**When** 呼叫 `GetEffective()`
**Then** 回傳的 `workspace.sync_interval` 為 `"600"`（settings.json 優先）；`workspace.sync_enabled` 則來自 env seed

---

### SP03 — redactSettings 遮蔽 workspace.git_token

**層級**：Unit

> **自動化**：`go test -run TestRedactSettings_WorkspaceGitToken ./...`

**Given** `RuntimeSettings.Workspace.GitToken` 設為非空字串
**When** 通過 `redactSettings()` 處理
**Then** 回傳的 `workspace.git_token` 值為 `"••••"`，原始 token 值不出現在輸出中

**When** `workspace.git_token` 為空字串時通過 `redactSettings()`
**Then** `workspace.git_token` 保持空字串（不遮蔽空值）

---

### SP04 — handleGetSettings 回傳完整 workspace 結構

**層級**：Integration

> **自動化**：`go test -run TestHandleGetSettings_WorkspaceFields ./...`

**Given** mock server 以含 workspace env vars 的 seed 啟動
**When** 發送 `GET /api/settings`
**Then** 回應 HTTP 200，body 為合法 JSON；`settings.workspace` 物件存在，包含 `sync_enabled`、`sync_interval`、`notify_channel` 欄位；`git_token` 值為 `"••••"`；`restart_required` 為 bool

---

## E2E-curl — 預設設定（AUTH_MODE=none，無 settings.json override）

### SP05 — GET /api/settings 回傳 workspace 欄位（env var 修復驗證）

**層級**：E2E-curl

**Given** Perch 以 `WORKSPACE_GIT_SYNC_ENABLED=true`、`WORKSPACE_GIT_SYNC_INTERVAL=300`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=alerts` 啟動，`AUTH_MODE=none`，無 settings.json 覆蓋
**When** 發送：
```bash
curl -s http://localhost:8081/api/settings | jq '.settings.workspace'
```
**Then** 回傳的 `workspace` 物件不為 `null`，且：
- `sync_enabled` 為 `true`
- `sync_interval` 為 `"300"`
- `notify_channel` 為 `"alerts"`
- `git_token` 為 `"••••"`（token 已設定時）或 `null`/缺失（token 未設定時）

**反向驗證**：若這三個 env vars 均未設定，`workspace` 欄位應為 `null` 或不含這些 key（不以預設值填充）：
```bash
curl -s http://localhost:8081/api/settings | jq '.settings.workspace'
```

---

### SP06 — GET /api/settings 基本結構驗證

**層級**：E2E-curl

**Given** Perch 正常啟動（`AUTH_MODE=none`）
**When** 發送：
```bash
curl -s http://localhost:8081/api/settings
```
**Then**
- 回應 HTTP 200，Content-Type 含 `application/json`
- body 頂層含 `settings` 與 `restart_required` 兩個 key
- `settings` 包含 `agent`、`auth`、`rate_limit` 子物件
- `agent.runtime` 值為 `"claude"` 或 `"opencode"`
- `auth.method` 值為 `"none"`、`"password"` 或 `"gitlab"` 之一
- `rate_limit.rpm` 為整數

---

### SP07 — PATCH /api/settings 修改欄位後 GET 回傳新值

**層級**：E2E-curl

**Given** Perch 正常啟動（`AUTH_MODE=none`）
**When** 先發送 PATCH 修改 `rate_limit.rpm`：
```bash
curl -s -X PATCH http://localhost:8081/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"rate_limit": {"rpm": 77}}'
```
接著再發送：
```bash
curl -s http://localhost:8081/api/settings | jq '.settings.rate_limit.rpm'
```
**Then** PATCH 回傳 HTTP 200，body 中 `rate_limit.rpm` 為 `77`；後續 GET 回傳同樣結果；`restart_required` 為 `false`（rate_limit 不需重啟）

**後置操作**：透過 `PATCH /api/settings` 將 `rate_limit.rpm` 改回原始值（或刪除 settings.json）以還原環境。

---

### SP08 — PATCH /api/settings 修改 restart-required 欄位

**層級**：E2E-curl

**Given** Perch 正常啟動（`AUTH_MODE=none`）
**When** 發送 PATCH 修改 `auth.method`（restart-required 欄位）：
```bash
curl -s -X PATCH http://localhost:8081/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"auth": {"method": "password", "password": "testpass999"}}'
```
**Then** PATCH 回應的 `restart_required` 為 `true`；後續 GET 的 `restart_required` 也為 `true`

**後置操作**：透過 `PATCH /api/settings` 將 `auth.method` 切回 `none`，再 `POST /api/admin/restart` 重啟並等待 server 回來以還原環境。

---

### SP09 — PATCH /api/settings 部分欄位修改不影響其他欄位

**層級**：E2E-curl

**Given** Perch 正常啟動，並已透過 PATCH 設定 `rate_limit.rpm=55` 及 `log.format="json"`
**When** 再次發送 PATCH，僅修改 `rate_limit.rpm=88`（不含 log 欄位）：
```bash
curl -s -X PATCH http://localhost:8081/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"rate_limit": {"rpm": 88}}'
```
**Then** GET 回傳 `rate_limit.rpm=88`，而 `log.format` 仍為 `"json"`（未修改的欄位不被清除）

---

### SP10 — PATCH /api/settings 修改 workspace 欄位並持久化

**層級**：E2E-curl

**Given** Perch 正常啟動（`AUTH_MODE=none`）
**When** 發送 PATCH 修改 workspace 設定：
```bash
curl -s -X PATCH http://localhost:8081/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"workspace": {"sync_enabled": false, "sync_interval": "120", "notify_channel": "test-ch"}}'
```
**Then** PATCH 回傳 HTTP 200；後續 GET 回傳 `workspace.sync_enabled=false`、`workspace.sync_interval="120"`、`workspace.notify_channel="test-ch"`；重啟 Perch 後再 GET，上述值仍保持（已寫入 settings.json）

---

### SP11 — POST /api/admin/restart 回傳 202 並觸發重啟

**層級**：E2E-curl

**Given** Perch 正常啟動，且 Docker restart policy 設為 `always` 或 `unless-stopped`
**When** 發送：
```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8081/api/admin/restart
```
**Then** 收到 HTTP 202；約 3-10 秒後 Perch 自動重啟，`GET /api/settings` 再次回傳 200（容器重新上線）

**反向驗證**：若 Docker restart policy 為 `no`，容器停止後 `GET /api/settings` 應回傳 connection refused（不是 502 或其他 Perch 錯誤），確認行為明確而非靜默失敗。

---

### SP12 — PATCH /api/settings 送入非法 JSON body

**層級**：E2E-curl

**Given** Perch 正常啟動（`AUTH_MODE=none`）
**When** 發送格式錯誤的 body：
```bash
curl -s -o /dev/null -w "%{http_code}" -X PATCH http://localhost:8081/api/settings \
  -H 'Content-Type: application/json' \
  -d 'not-json'
```
**Then** 回傳 HTTP 400；現有設定不受影響，後續 GET 仍正常

---

## E2E-browser — 預設設定（AUTH_MODE=none）

### SP13 — Settings 面板開啟與分頁渲染

**層級**：E2E-browser

**Given** Perch 以 `AUTH_MODE=none` 啟動，使用者開啟 `/chat`
**When** 點擊 sidebar 底部的齒輪圖示（Settings 入口）觸發 `perch:open-settings` 事件
**Then**
- 頁面中央出現 Settings modal dialog
- dialog 頂部顯示「Settings」標題
- 上方有五個分頁標籤：General、Auth、Integrations、Workspace、Advanced
- 預設顯示 General 分頁內容
- 分頁切換流暢，點擊 Auth 分頁可看到 Auth Method radio 選項（none / password / gitlab）

---

### SP14 — General 分頁顯示正確的 Agent Runtime 與 Args

**層級**：E2E-browser

**Given** Perch 以 `AGENT_RUNTIME=claude` 啟動，Settings 面板開啟
**When** 切換到 General 分頁
**Then**
- Agent Runtime 顯示兩個 radio 選項（claude / opencode），目前選中的為 `claude`
- Agent Args 欄位顯示從 env var 讀取的值（若未設定則為空白）
- Rate Limit (RPM) 欄位顯示目前有效值（預設 10）

---

### SP15 — Workspace 分頁從 env var 正確顯示欄位值

**層級**：E2E-browser

**Given** Perch 以 `WORKSPACE_GIT_SYNC_ENABLED=true`、`WORKSPACE_GIT_SYNC_INTERVAL=300`、`WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=alerts` 啟動，Settings 面板開啟
**When** 切換到 Workspace 分頁
**Then**
- Git Sync checkbox 顯示已勾選（啟用）
- Sync Interval 欄位顯示 `300`（而非預設的 `60s`）
- Notify Discord Channel 欄位顯示 `alerts`
- Git Token 欄位顯示 `••••`（token 已設定）或為空白（token 未設定），不顯示明文 token

**反向驗證**：若以上 env vars 均未設定，Git Sync checkbox 應顯示未勾選，Sync Interval 顯示 `60s`（前端預設值），Notify Channel 為空白。

---

### SP16 — Save 按鈕儲存成功顯示 Saved toast

**層級**：E2E-browser

**Given** Settings 面板開啟，General 分頁可見
**When** 修改 Rate Limit (RPM) 欄位為一個新數值，點擊 Save 按鈕
**Then**
- 按鈕短暫顯示「Saving…」（disabled 狀態）
- 儲存完成後 footer 左側出現藍色「Saved」toast 訊息
- 約 3 秒後 toast 自動消失
- `restart_required` 為 `false`（Rate Limit 不需重啟），按鈕恢復為藍色「Save」

---

### SP17 — 修改 restart-required 欄位後按鈕變為 Save & Restart

**層級**：E2E-browser

**Given** Settings 面板開啟，Auth 分頁可見，目前 `restart_required=false`
**When** 點擊 Auth Method，切換為 `password`，輸入新密碼，點擊 Save
**Then**
- 儲存後 footer 右側按鈕文字變為「Save & Restart」，樣式顯示橘色警示外框
- 再次點擊「Save & Restart」並確認 dialog
- 面板顯示「Restarting…」，按鈕 disabled
- Perch 重啟後瀏覽器自動導向 `/chat`（server 回來後自動 reload）

**後置操作**：重啟後透過 Settings 面板將 Auth Method 切回 `none` 並 Save & Restart，以還原環境。

---

### SP18 — 關閉 Settings 面板（backdrop 點擊與 × 按鈕）

**層級**：E2E-browser

**Given** Settings 面板已開啟
**When** 點擊 dialog 外的半透明遮罩（backdrop）
**Then** Settings 面板關閉，回到 `/chat` 主畫面，無任何錯誤訊息

**When** 重新開啟 Settings 面板後，點擊右上角的 × 按鈕
**Then** Settings 面板關閉，效果與點擊 backdrop 相同

---

### SP19 — server 無法連線時 UI 顯示錯誤而非崩潰

**層級**：E2E-browser

**Given** 使用者在瀏覽器中已開啟 `/chat`，然後 Perch container 被手動停止（模擬 container down）
**When** 使用者點擊 Settings 圖示，嘗試開啟 Settings 面板
**Then**
- Settings 面板仍然開啟（或靜默失敗不開啟）
- 若面板開啟：各欄位為空白或保留上一次載入的快取值，不出現 JavaScript 未捕獲例外
- 點擊 Save 時，footer 顯示紅色「Network error」或「Save failed」錯誤訊息
- 頁面不出現白畫面（white screen of death）或 React error boundary

**When** Perch container 重新啟動後，使用者重新整理頁面並開啟 Settings 面板
**Then** Settings 面板正常載入，顯示最新設定值

---

## 已知限制

### 限制：AUTH_MODE=none 時 /api/settings 無需 admin cookie

目前測試環境 `AUTH_MODE=none` 且無 `ADMIN_TOKEN`，因此 `/api/settings` 端點對所有請求均開放，不驗證 admin 身份。RS01 中的「非 admin 應收到 HTTP 403」行為，在此環境下無法驗證，需切換至 password 或 gitlab 模式才能測試。
