## Why

perch 容器內的 Claude（Discord agent）目前沒有瀏覽器自動化能力，無法處理「需要登入 / 點選 / 抓 PDF」這類網頁任務（首例：自動下載台北富邦信用卡帳單 PDF）。

host (Mac) 端的 `tests/chrome-agent.sh` + `chrome-cdp` skill 是現有方案，但**只能跑在有 GUI 的 host**，container 內無 GUI、無 VNC。需要另一套 container 內可用、headless 但仍能處理 CAPTCHA / OTP 等人為介入場景的方案。

關鍵洞察：perch 既然是 Discord IM agent，**Discord 訊息 + 截圖**本身就是天然的 human-in-the-loop 介面，比 VNC / X11 forward 更適合這個架構。

## What Changes

- **Dockerfile**：runtime stage 新增 Chromium + 系統 lib + CJK 字型 + Playwright npm 套件
- **MCP 設定**：在 `claude/` runtime config 加入 `@playwright/mcp`，預設 `--headless`，掛 `--user-data-dir=/data/playwright/profile`
- **新 skill**：`claude/skills/browser-automation/` — 教 Claude 何時用 browser、CAPTCHA retry 模式、Discord 截圖求助模式、storageState 規範、敏感資料注入
- **Volume 規劃**：新增 `/data/playwright/`（profile + downloads + storage-state）、`/data/secrets/`（敏感資料，chmod 600）
- **Mac 端 bootstrap script**：`tests/playwright-login.sh` — 在 Mac 跑 headed Playwright 互動登入後 dump `storageState.json`，用於需要複雜首次登入的網站
- **首個 use case skill**：`claude/skills/finance-fubon-statement/` — 自動下載富邦信用卡帳單 PDF，驗證整套架構

## Capabilities

### New Capabilities

- `browser-automation`：container 內 headless Playwright + Discord 截圖介入模式，含敏感資料注入規範與 storageState 持久化
- `finance-fubon-statement`：自動抓取 Gmail 內富邦帳單通知 → 開啟下載頁 → Claude 解析 CAPTCHA（失敗向 Doro 求助）→ 注入身分證號/生日 → 下載加密 PDF 歸檔

### Modified Capabilities

<!-- none -->

## Impact

- `Dockerfile`：runtime stage 新增 ~270MB（Chromium binary + system libs + CJK 字型）
- `claude/skills/`：新增 `browser-automation/` 與 `finance-fubon-statement/` 兩個目錄
- `entrypoint.sh`：可能需新增 `/data/playwright/profile` 與 `/data/secrets/` 的目錄初始化（若不存在）
- 新增 host 端 script：`tests/playwright-login.sh`
- 既有 `tests/chrome-agent.sh` + chrome-cdp skill **不變**：host 端互動 debug 仍用此方案，與 container 內 Playwright 並存
- README / DEVELOPMENT.md：新增 browser-automation 章節說明
