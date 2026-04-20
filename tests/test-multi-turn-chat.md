# Multi-Turn Chat 測試案例

> 功能：multi-turn-chat
> 規格來源：`openspec/changes/multi-turn-chat/specs/multi-turn-chat/spec.md`
> 撰寫日期：2026-04-19

---

## 測試層級說明

| 層級 | 說明 |
|------|------|
| **Unit** | Go unit test，直接測試 `GetRecentHistory`、`buildPrompt` 等函式，注入 fake DB |
| **Integration** | 啟動 in-process HTTP server（`httptest`），使用真實 SQLite（in-memory），curl/Go HTTP client 呼叫 |
| **E2E-curl** | 啟動真實 binary，curl 驗證 HTTP 行為，DB 需預先填入測試資料 |
| **E2E-browser** | 瀏覽器手動操作，驗證前端渲染行為 |

---

## MT02 — 24 小時以外的 session 不注入

**層級**：Unit / Integration

**Given** 使用者只有一筆超過 24 小時前的對話記錄，沒有任何近期記錄
**When** 使用者送出新的查詢：「你記得我上次說了什麼？」
**Then** Agent 從空白的上下文開始回應，不帶任何過去的對話記錄

---

## MT03 — 無歷史記錄時不加前置

**層級**：Unit / Integration

**Given** 這是一個全新使用者，從未有過任何對話
**When** 使用者送出第一則訊息：「你好，我是新使用者」
**Then** Agent 正常回應，不含任何歷史前置內容

---

## MT04 — 歷史最多 20 筆

**層級**：Unit

**Given** 使用者在過去 24 小時內已有 25 筆對話記錄
**When** 使用者送出新的查詢
**Then** Agent 最多注入最近 20 筆對話，最舊的 5 筆被排除

---

## MT05 — new_conversation=true 略過歷史查詢

**層級**：Integration

**Given** 使用者有近期的對話歷史記錄
**When** 使用者帶著「開啟新對話」的旗標送出查詢
**Then** Agent 從空白上下文開始，不帶任何先前的對話記錄（歷史查詢未被執行）

---

## MT06 — 省略 new_conversation 欄位時行為正常

**層級**：Integration

**Given** 使用者有近期的對話記錄
**When** 使用者送出普通查詢（未附加任何特殊旗標）
**Then** Server 正常查詢近期 session 並注入歷史（與 MT01 行為一致）

---

## MT07 — 前端顯示多輪對話串

**層級**：E2E-browser（JS mock inject，無需真實 agent）

**Given** 使用者已登入 chat 頁面
**When** 使用者依序送出「訊息 A」，等待回應後，再送出「訊息 B」
**Then**
- 畫面上同時顯示「訊息 A」和「回應 A」、「訊息 B」和「回應 B」
- 每個新回應 append 至底部，先前的訊息不消失
- 對話串可垂直捲動

---

## MT08 — 新對話按鈕清除前端並重置歷史

**層級**：E2E-browser

**Given** 使用者已送出幾則訊息，畫面上有對話內容
**When** 使用者點擊「New conversation」按鈕，然後送出新的查詢
**Then**
- 點擊後，畫面上的舊訊息立即清空
- 新的查詢以全新上下文送出，Agent 不帶任何歷史記錄開始回應

---

## MT09 — Discord 查詢不注入對話歷史

**層級**：Unit

**Given** 使用者透過 Discord 傳送訊息給 Bot
**When** Discord handler 建立 session
**Then** session 不帶任何對話歷史，每次 Discord 訊息都從空白上下文開始

---

## MT10 — Scheduler 查詢不注入對話歷史

**層級**：Unit

**Given** 排程器在設定的時間觸發一個任務
**When** 排程器建立 session
**Then** session 不帶任何對話歷史，排程任務從空白上下文開始執行

---

## MT11 — 歷史格式正確

**層級**：Unit

**Given** 使用者有 2 筆近期對話記錄
**When** `buildPrompt` 函式組裝 prompt
**Then** 注入的歷史前置格式符合規範：
```
<conversation_history>
User: <turn 1 query>
Assistant: <turn 1 response>
...
</conversation_history>

<current query>
```
每對 User/Assistant 換行分隔，整個區塊以 `<conversation_history>` / `</conversation_history>` 包裹，後接一個空行再接當前查詢。

---

## MT12 — 管理員歷史頁面不受影響

**層級**：E2E-curl / Integration

**Given** 使用者已完成數輪對話
**When** 管理員查看歷史記錄頁面
**Then**
- 每一輪查詢以獨立記錄顯示
- 儲存的查詢內容只有使用者原始輸入，不含歷史前置（歷史注入只在送給 agent 前發生，不影響儲存）
