# 虛擬鍵盤 測試案例

> 功能：virtual-keyboard
> 涵蓋範圍：裝置類型偵測、鍵盤展收、原生鍵盤彈出時的 viewport 調整。
> 撰寫日期：2026-04-20

---

## T08 — 虛擬鍵盤（電腦瀏覽器）

**層級**：E2E-browser

**Given** 使用者在電腦瀏覽器開啟 `http://localhost:8080/terminal`
**When** 頁面載入完成
**Then** 畫面右下角顯示一個 ⌨ 浮動按鈕，虛擬鍵盤列預設收合

**When** 使用者點擊 ⌨ 按鈕
**Then** 底部展開虛擬鍵盤列，顯示 Esc、↑、↓、←、→、▼ 等按鍵

---

## T08b — 虛擬鍵盤（手機瀏覽器）

**層級**：E2E-browser（手機裝置）

**Given** 使用者用手機瀏覽器開啟 Perch
**When** 頁面載入完成
**Then** 底部自動展開虛擬鍵盤列（不需點擊 ⌨），顯示 Esc、↑、↓、←、→、▼ 等按鍵

**When** 使用者點擊 ▼ 按鈕
**Then** 虛擬鍵盤列收合，改顯示 ⌨ 浮動按鈕

**When** 使用者點擊方向鍵或 Esc
**Then** terminal 中游標移動或收到 Escape，行為符合預期

---

## T08c — 原生鍵盤彈出時的畫面調整

**層級**：E2E-browser（手機裝置）

**Given** 使用者在手機上使用 Perch terminal
**When** 使用者點擊 terminal 觸發原生螢幕鍵盤彈出
**Then** terminal 自動縮小至剩餘可視區域，底部游標行保持可見，不被鍵盤遮住
