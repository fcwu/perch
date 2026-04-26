# 基礎啟動與前端 測試案例

> 功能：basic-startup
> 涵蓋範圍：伺服器啟動模式、前端初始渲染、PTY 輸出顯示、Build time 標記。
> 撰寫日期：2026-04-20

---

## E2E-curl — 預設設定（AUTH_METHOD=none）

### T33 — Build Time 顯示在啟動 Log

**層級**：E2E-curl

**Given** 使用 Docker 執行 perch 官方 image
**When** container 啟動並輸出第一批 log
**Then** log 中可見 `built=` 欄位，值為 ISO 8601 格式的 UTC 時間，例如：
```
time=... level=INFO msg="perch listening" addr=:8080 auth=none built=2026-04-12T10:30:00Z
```

**反向驗證**：本機直接 `go build`（不帶 ldflags）執行，`built=unknown` 應出現在 log 中。

---

## E2E-browser — 預設設定（AUTH_METHOD=none）

### T01 — 啟動（none 模式）

**層級**：E2E-browser

**Given** Perch 以 `AUTH_MODE=none` 啟動
**When** 使用者在瀏覽器開啟 `http://localhost:8080`
**Then** 瀏覽器收到 302 重導向至 `/chat`；`/chat` 頁面正常載入，顯示左側 sidebar 與 chat 輸入區的 Chat UI 版面，不出現任何錯誤頁面

**反向驗證**：若改用 `https://localhost:8080`，瀏覽器應顯示無法連線（非 TLS server）。

---

### T01b — Terminal 路由（none 模式）

**層級**：E2E-browser

**Given** Perch 以 `AUTH_MODE=none` 啟動（auth=none 模式下 /terminal 無需 admin cookie）
**When** 使用者在瀏覽器開啟 `http://localhost:8080/terminal`
**Then** 頁面正常載入，顯示 xterm.js terminal 畫面，不出現任何錯誤頁面或重導向

---

### T02 — 前端載入（Terminal）

**層級**：E2E-browser

**Given** Perch 已正常啟動
**When** 使用者開啟 `http://localhost:8080/terminal`
**Then**
- 看到黑色 terminal 畫面
- 畫面底部顯示虛擬鍵盤列
- 狀態列顯示使用者名稱與工作目錄

---

### T03 — Terminal 輸出（PTY 串流）

**層級**：E2E-browser

**Given** 使用者已完成 T02 的頁面載入
**When** 使用者等待幾秒
**Then** terminal 畫面中出現 Claude Code 的啟動歡迎訊息

---

### T04 — Terminal 輸入

**層級**：E2E-browser

**Given** terminal 畫面已顯示且 Claude Code 已就緒
**When** 使用者點擊 terminal 畫面，輸入任意文字並按 Enter
**Then** 輸入的文字出現在 terminal 中，Claude Code 收到並開始回應

---

### T17 — 首次開啟 Web UI Terminal 填滿畫面

**層級**：E2E-browser

**Given** Perch 已啟動（任何認證模式均可）
**When** 使用者開啟一個全新瀏覽器分頁，直接輸入 `http://localhost:8080/terminal`
**Then**
- Terminal 黑色區域填滿整個視窗，上下左右無明顯空白邊緣
- 不需手動縮放或重整頁面
- 字元排列正確對應視窗大小

**反向驗證**：調整瀏覽器視窗大小後，terminal 應自動重新填滿，不出現黑色空白條。
