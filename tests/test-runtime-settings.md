# Runtime Settings 測試案例

> 功能：runtime-settings
> 規格來源：`openspec/changes/chat-ui-session-settings/specs/runtime-settings/spec.md`
> 撰寫日期：2026-04-24

---

## 測試層級說明

| 層級 | 說明 |
|---|---|
| **Unit** | Go unit test，直接測試 SettingsManager |
| **E2E-curl** | 啟動真實 binary，curl 驗證 HTTP 行為 |
| **E2E-browser** | 瀏覽器操作驗證前端 Settings panel |

---

## RS01 — GET /api/settings（敏感欄位遮蔽）

**層級**：E2E-curl

**Given** Perch 啟動，`DISCORD_BOT_TOKEN` 設為非空值
**When** Admin 發送 `GET /api/settings`（帶 admin cookie）
**Then** 回傳 JSON，`discord.bot_token` 值為 `"••••"`；`restart_required` 為 bool

**反向驗證**：非 admin 發送 `GET /api/settings` → HTTP 403

---

## RS02 — PATCH 即時生效欄位（rate_limit_rpm）

**層級**：E2E-curl

**Given** Perch 以 `RATE_LIMIT_RPM=10` 啟動
**When** Admin 發送 `PATCH /api/settings` with `{"rate_limit": {"rpm": 2}}`
**Then** 立刻生效：在短時間內送 3 次請求超過限速即收到 HTTP 429；`/data/settings.json` 包含新值

---

## RS03 — PATCH restart-required 欄位回傳 restart_required=true

**層級**：E2E-curl

**Given** Perch 正常啟動
**When** Admin 發送 `PATCH /api/settings` with `{"auth": {"method": "password"}}`
**Then** 回傳 JSON 中 `restart_required` 為 `true`；`GET /api/settings` 的 `restart_required` 也為 `true`

---

## RS04 — Env var 重啟後生效（非 UI override）

**層級**：E2E-curl

**Given** `settings.json` 不存在或不含 `rate_limit.rpm`；`RATE_LIMIT_RPM=5` 在 .env
**When** 重啟容器
**Then** `GET /api/settings` 回傳中 `rate_limit.rpm` 有效值為 5（從 env var 讀取）

---

## RS05 — UI 覆蓋優先於 Env var

**層級**：E2E-curl

**Given** `RATE_LIMIT_RPM=5`，且已透過 PATCH 將 `rate_limit.rpm` 設為 20
**When** 重啟容器（`settings.json` 仍存在）
**Then** `GET /api/settings` 的 `rate_limit.rpm` 為 20（settings.json 優先）

---

## RS06 — Settings Panel 開啟與儲存

**層級**：E2E-browser

**Given** Admin 使用者在瀏覽器開啟 `/chat`
**When** 點擊 sidebar 底部的 ⚙ Settings
**Then** Settings panel 從左側滑出，載入當前設定；修改 Rate Limit 欄位後點 Save → 顯示「Saved」toast

---

## RS07 — Restart Container 按鈕

**層級**：E2E-browser

**Given** Settings panel 開啟，且 footer 顯示橘色「Save & Restart」按鈕（表示已修改需重啟的欄位）
**When** 點擊橘色的 ⚠ Restart Container 按鈕，確認 dialog
**Then** 顯示「Restarting…」訊息；容器重啟後設定生效

---

## RS08 — 非 Admin 看不到 Settings 入口

**層級**：E2E-browser

**Given** Multi-user 模式，非 admin 使用者登入
**When** 查看 sidebar
**Then** 底部不顯示 ⚙ Settings、🖥 Terminal、🛡 Admin 連結

---
