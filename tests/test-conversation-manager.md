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

## CM09 — 點擊 conversation 不觸發整頁重新載入（SPA 切換）

**層級**：E2E-browser

**Given** 使用者在 `/chat`，sidebar 顯示多個 conversations
**When** 使用者點擊其中一個 conversation 項目
**Then** sidebar 將該項目標示為選中狀態，chat area 切換為該對話的內容，URL 更新為 `/chat?id=<uuid>`；瀏覽器分頁標題不閃爍或消失，表示頁面未整頁重新載入

---

## CM10 — 直接開啟帶 id 的 URL 顯示歷史訊息

**層級**：E2E-browser

**Given** 已存在一個有歷史訊息的 conversation（透過先前的對話建立），其 id 為 `<uuid>`
**When** 使用者在瀏覽器網址列直接輸入 `/chat?id=<uuid>` 並按 Enter
**Then** 頁面載入後，chat area 呈現該對話的歷史訊息：使用者發出的訊息顯示在右側，助理回覆顯示在左側（包含回覆中的圖片，若有的話）；使用者不需要重新送出任何訊息

---

## CM11 — 強制重新整理後歷史訊息仍保留

**層級**：E2E-browser

**Given** 使用者已在 `/chat?id=<uuid>` 頁面，chat area 可見歷史訊息（含文字與圖片）
**When** 使用者按下 F5 或 Ctrl+Shift+R 執行強制重新整理
**Then** 頁面重載完成後，chat area 仍顯示相同的歷史訊息（包含助理回覆中的圖片，無破圖）；URL 維持 `/chat?id=<uuid>`，使用者不需要任何額外操作

---
