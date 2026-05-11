# Conversation Manager 測試案例

> 功能：conversation-manager
> 規格來源：`openspec/changes/chat-ui-session-settings/specs/conversation-manager/spec.md`
> 撰寫日期：2026-04-24

---

## 測試層級說明

| 層級 | 說明 |
|---|---|
| **Unit** | Go unit test，直接測試 store 方法，用 in-memory SQLite |
| **E2E-curl** | 啟動真實 binary，curl 驗證 HTTP 行為 |
| **E2E-browser** | 瀏覽器操作驗證前端行為 |

---

## CM01 — 新建 conversation（無 conversation_id）

**層級**：E2E-curl

**Given** Perch 以 `AUTH_METHOD=none` 啟動，DB 已初始化
**When** 發送 `POST /api/chat` 不帶 `conversation_id`
**Then** 回傳 JSON 包含 `conversation_id` 欄位（非空 UUID）；`GET /api/conversations` 回傳該 conversation

---

## CM02 — 列出 conversations

**層級**：E2E-curl

**Given** 已有 2 個 conversations
**When** 發送 `GET /api/conversations`
**Then** 回傳 JSON array，長度 2，每筆含 `{id, title, created_at, updated_at}`，依 `updated_at DESC` 排序

---

## CM03 — 刪除 conversation

**層級**：E2E-curl

**Given** 已有 conversation，id 為 `<uuid>`
**When** 發送 `DELETE /api/conversations/<uuid>`
**Then** 回傳 HTTP 204；再次 `GET /api/conversations` 該 conversation 不再出現

---

## CM04 — 刪除不存在的 conversation

**層級**：E2E-curl

**Given** 不存在的 conversation id
**When** 發送 `DELETE /api/conversations/nonexistent-id`
**Then** 回傳 HTTP 404

---

## CM05 — Sidebar 顯示 conversation 列表

**層級**：E2E-browser

**Given** 已有多個 conversations，瀏覽器開啟 `http://localhost:8080/chat`
**When** 頁面載入完成
**Then** 左側 sidebar 顯示 conversation 列表，依 Today/Yesterday/Last 7 Days 分組

---

## CM06 — 點擊 conversation 切換

**層級**：E2E-browser

**Given** Sidebar 顯示多個 conversations
**When** 點擊某個 conversation
**Then** URL 更新為 `/chat?id=<uuid>`，chat area 顯示該對話（訊息區清空，等待新輸入）

---

## CM07 — Hover 顯示刪除按鈕

**層級**：E2E-browser

**Given** Sidebar 顯示 conversation 列表
**When** 滑鼠 hover 在某 conversation 上
**Then** 右側出現 ✕ 刪除按鈕；點擊後該 conversation 從列表消失

---

## CM08 — 新對話後 Sidebar 自動更新

**層級**：E2E-browser

**Given** 開啟 `/chat`（新對話）
**When** 輸入第一個訊息並送出
**Then** URL 更新為 `/chat?id=<uuid>`；sidebar 自動出現新 conversation 項目

---

## CM09 — 點擊 conversation 切換為 SPA（不觸發整頁重新載入）

**層級**：E2E-browser

**Given** Sidebar 顯示多個 conversations，目前在 `/chat`
**When** 點擊某個 conversation 連結
**Then** URL 更新為 `/chat?id=<uuid>`（透過 `history.pushState`），頁面**不**觸發整頁重新載入（`window.performance.navigation.type` 保持 0，或用 JS 旗標驗證 DOM 未 destroy）

**驗證方式（curl/browser）**：
```js
// 在頁面上設置旗標
window.__noReload = true;
// 點擊 conversation link
// 點擊後確認旗標仍存在（未 reload）
window.__noReload === true  // → true = SPA OK
```

---

## CM10 — 開啟帶有 id 的 URL 時自動載入歷史訊息

**層級**：E2E-browser

**Given** conversation `<uuid>` 已有 2 筆 query_sessions（query + response 都有值）
**When** 瀏覽器直接打開 `/chat?id=<uuid>`
**Then** chat area 顯示所有歷史訊息（user bubble + assistant bubble），不需重新送出訊息

**驗證方式（curl）**：
```bash
# 確認 API 有資料
curl -s http://localhost:8081/api/conversations/<uuid>/messages | jq '.messages | length'
# → 應 >= 1

# 用 browser 工具確認 DOM 有 bubble 元素
```

---

## CM11 — 重新載入頁面後歷史訊息保留

**層級**：E2E-browser

**Given** 已在 `/chat?id=<uuid>` 且頁面顯示歷史訊息
**When** 執行 hard reload（F5 / Ctrl+R / `Page.reload` with `ignoreCache: true`）
**Then** 重載後頁面仍顯示相同的歷史訊息；URL 維持 `/chat?id=<uuid>`

---
