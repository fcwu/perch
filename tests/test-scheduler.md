# 排程器 測試案例

> 功能：scheduler
> 涵蓋範圍：排程 CRUD、資料持久化路徑。
> 撰寫日期：2026-04-20

---

## T05 — 排程器 列出

**層級**：E2E-browser

**Given** Perch 已啟動，workspace 中可能有或沒有既有排程
**When** 使用者在 terminal 中詢問 Claude：「目前有哪些排程？」
**Then** Claude 回覆目前的排程清單；若無排程，顯示「目前沒有排程」；有排程時每筆資料包含時間和說明

---

## T06 — 排程器 新增

**層級**：E2E-browser

**Given** Perch 已啟動
**When** 使用者在 terminal 中告訴 Claude：「每天早上 9 點提醒我喝水，重複執行」
**Then**
- Claude 確認排程已建立，並回覆排程的時間與說明
- 系統開始追蹤這個排程（可透過「目前有哪些排程？」確認）
- 排程資料立即生效，不需重啟

---

## T07 — 排程器 刪除

**層級**：E2E-browser

**Given** 已存在一個喝水提醒的排程（如 T06 建立的）
**When** 使用者在 terminal 中告訴 Claude：「刪除剛才那個喝水提醒」
**Then**
- Claude 確認排程已刪除
- 詢問目前排程時，該排程不再出現

---

## T27 — 排程資料存入 workspace 隱藏目錄

**層級**：E2E-browser

**Given** Perch 以 workspace volume 掛載的方式執行
**When** 使用者透過 Claude 設定一個排程，然後重啟 container
**Then**
- 重啟後詢問排程，該排程仍然存在
- 排程資料儲存於 workspace 的 `.perch/` 目錄，不影響工作區的其他檔案
