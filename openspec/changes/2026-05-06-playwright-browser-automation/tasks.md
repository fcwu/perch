# Tasks

## 1. Dockerfile：安裝 Chromium + Playwright + 字型

- [x] 1.1 runtime stage (`FROM ubuntu:24.04`) 在現有 `apt-get install` 加入 `fonts-noto-cjk fonts-noto-cjk-extra`
- [x] 1.2 npm 安裝步驟新增 `@playwright/mcp` 與 `playwright`
- [x] 1.3 新增 `RUN npx playwright install --with-deps chromium`
- [x] 1.4 確認最終 image 大小：baseline 802 MB → new 1,503 MB，**+701 MB**（Chromium ~270 MB + system deps ~180 MB + CJK fonts ~250 MB）；已記錄至 DEVELOPMENT.md
- [x] 1.5 local build 驗證：`npx @playwright/mcp --version` → `0.0.73`；chromium binary 存在於 `/root/.cache/ms-playwright/chromium-1217/`

## 2. MCP Config：把 Playwright 接到 Claude

- [x] 2.1 找出 perch container 內 Claude 載入的 MCP config 路徑（`~/.claude.json`，由 entrypoint.sh 的 jq 區塊 seed）
- [x] 2.2 加入 `playwright` server entry，args: `["-y", "@playwright/mcp", "--headless", "--user-data-dir=/data/playwright/profile", "--browser=chromium"]`（profile 為共用，per-conv-id 隔離為未來 open question）
- [x] 2.3 entrypoint.sh 確保 `/data/playwright/{profile,downloads,state}`、`/data/secrets/`、`/data/finance/` 目錄存在
- [x] 2.4 驗證：`/home/perchuser/.claude.json` 內 `.mcpServers.playwright` 正確 seed（command: npx, args 含 --headless --browser=chromium --user-data-dir）；`/data/playwright/{profile,downloads,state}`、`/data/secrets/`(0700)、`/data/finance/` 全數由 entrypoint 建立

## 3. 新 Skill：browser-automation

- [x] 3.1 建立 `claude/skills/browser-automation/SKILL.md`，內容含：
  - frontmatter (`name`, `description`, 觸發條件)
  - 何時使用 / 何時不該使用（不要拿來做爬蟲量產）
  - 核心 MCP tools 速查（navigate / screenshot / click / type / select / wait_for）
  - **CAPTCHA 處理 SOP**：先截圖自己讀，回 4 碼填入；表單錯誤訊息出現 → 點重新產生 → 重試；連 3 次失敗 → 截圖丟 Discord 求 Doro
  - **Discord 介入 SOP**：何時該停下、訊息範本（含截圖 + 具體問題）
  - **storageState 規範**：路徑 `/data/playwright/state/<site>.json`；不存在時不要嘗試自己登入，回報 Doro 走 Mac bootstrap
  - **敏感資料注入規範**：Claude 不直接讀 secrets；用 `browser_type` 搭配 shell 子命令注入；對話 log 不出現明文
  - 失敗 / 重試上限規則
- [x] 3.2 自我 review：規範含具體範例和 SOP，可直接執行

## 4. Use Case Skill：finance-fubon-statement

- [x] 4.1 建立 `../../.claude/skills/finance-fubon-statement/SKILL.md`（個人 skill，放在 mykb/.claude，不進 code repo）：
  - 觸發：Doro 說「抓 fubon 帳單 / 富邦信用卡帳單」
  - 步驟 1：`gws gmail users messages list --params '{"q":"from:fubon 信用卡帳單","maxResults":5}'` 找最新帳單 email
  - 步驟 2：`gws gmail +read --id <ID> --html` 抓 body
  - 步驟 3：grep 出 `下載本期帳單(PDF)` 的 href
  - 步驟 4：用 browser-automation skill 開頁面、注入 ID + 生日（從 `/data/secrets/fubon.json`）、填 CAPTCHA、點下載
  - 步驟 5：存檔到 `/data/finance/fubon/YYYY-MM.pdf`（從 email subject 解析年月）
  - 步驟 6：成功後回報 Doro，附加密 PDF 大小 + 路徑
- [x] 4.2 secrets schema 寫在 SKILL.md 的 Prerequisites 區塊（不建立 /data/secrets/fubon.example.json 實體檔，schema 在 skill doc 內已有）

## 5. Mac 端 Bootstrap Script

- [x] 5.1 `tests/playwright-login.sh <site>`：
  - 啟動 Mac 端 headed Playwright（chromium）
  - 開啟對應網站登入頁（`<site>` 對應 URL 表寫在腳本內）
  - Doro 手動登入完關閉視窗
  - dump `storageState.json`
  - `scp` 到 `${PLAYWRIGHT_REMOTE_HOST}:/data/playwright/state/<site>.json`（host 可從 `tests/.env.<device>.md` 讀 `PLAYWRIGHT_REMOTE_HOST`）
- [x] 5.2 DEVELOPMENT.md 補「storageState Bootstrap 流程」說明

## 6. 文件

- [x] 6.1 DEVELOPMENT.md：不額外新增章節（看 code 已知），image size delta (+701 MB) 也移除
- [x] 6.2 README.md 新增「瀏覽器自動化」bullet（headless Chromium + Discord 截圖介入模式）

## 7. 驗收

- [ ] 7.1 在 home-auto container 跑：「抓 fubon 4 月帳單」，全程不需要 Doro 介入（驗證碼 Claude 自解）→ PDF 落到 `/data/finance/fubon/2026-04.pdf`
- [ ] 7.2 故意把 secrets 拿掉，確認 Claude 給出「請先補 secrets」的明確錯誤訊息
- [ ] 7.3 故意關 Discord，確認 Claude 在解析驗證碼失敗時不會 hang，會 timeout 並回報

## 不在本 change 範圍

- 多 user / multi-tenant secrets 隔離（現階段只 Doro 一個用戶）
- cron 化每月自動抓（之後另立 change，用 local-schedule skill）
- 其他銀行 / 帳單來源（建立樣板後遇到再加）
- Firefox / WebKit 支援
- noVNC / Xvfb 退路（被擋了再做）
