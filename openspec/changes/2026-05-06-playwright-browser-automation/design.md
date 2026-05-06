# Playwright Browser Automation — Design

## 目標

讓 perch container 內的 Claude 透過 MCP 自主操作瀏覽器，預設 headless，遇到無法自動化的步驟（驗證碼讀不出、OTP、非預期跳窗）時，透過 **Discord 截圖 + 文字** 由 Doro 即時介入，不依賴 VNC / X11。

## 架構

```
┌─ perch container (home-auto) ────────────────────────────┐
│                                                          │
│  Claude (claude-code CLI)                                │
│   │                                                      │
│   ├─ MCP: @playwright/mcp --headless                     │
│   │     --user-data-dir=/data/playwright/profile         │
│   │     --browser=chromium                               │
│   │                                                      │
│   ├─ Skill: browser-automation                           │
│   │   - 何時用 / CAPTCHA retry / 截圖求助 / 敏感資料注入 │
│   │                                                      │
│   └─ Skill: finance-fubon-statement (use case)           │
│                                                          │
│  Volume mounts:                                          │
│   /data/playwright/profile       ← cookies + 養 fingerprint│
│   /data/playwright/downloads     ← PDF 落地              │
│   /data/playwright/state/<site>.json ← storageState      │
│   /data/secrets/<site>.json      ← 帳密、ID、生日 (600)  │
│   /data/finance/                 ← 帳單歸檔              │
│                                                          │
└──────────────────────────────────────────────────────────┘
              ↑                              ↓
              │ Discord IM                   │ HTTPS
              │                              │
        [Doro on phone/laptop]        [外部網站]
```

## 關鍵設計決策

### 1. Headless + user-data-dir，不上 VNC

| 方案 | 採用？ | 原因 |
|---|---|---|
| Headless + Discord 截圖介入 | ✅ | perch 既有 Discord 通道；手機也能介入；零基礎設施 |
| noVNC | ❌ | 多開 port、認證、複雜度高 |
| X11 forwarding | ❌ | 跨 SSH 慢、Mac 要 XQuartz、不適合行動介入 |
| Xvfb（純 framebuffer headed） | 🟡 退路 | headless 被 bot detection 擋時的 fallback |

`--user-data-dir` 持久化整個 Chrome profile（cookies + localStorage + service workers + 養成 fingerprint），對抗 bot detection 比 storageState 更扎實。

### 2. Human-in-the-loop 模式

Claude 遇到不確定步驟時，**主動發訊息給 Doro**：
- 截圖（MCP `browser_take_screenshot`）
- 文字提問：「驗證碼是 7319 嗎？我不太確定第一位」
- 等待 Doro 回覆
- 收到後 `browser_type` 注入

實作面：perch Discord agent 本來就是雙向對話，不需要額外機制。skill 文件教 Claude 何時應該停下來問。

### 3. 敏感資料注入（不暴露給 Claude）

**問題**：身分證號、密碼、生日都是 PII，不能寫進 skill / repo / log。

**解法**：`/data/secrets/<site>.json` 由 host 掛入，chmod 600。Skill 教 Claude **不直接讀取**，而是用 `browser_type` 配合 shell 子命令：

```
browser_type(selector="#id", text=$(jq -r .id /data/secrets/fubon.json))
```

對 Claude 而言看到的是「已從 secrets 注入欄位」，不會把明文 PII 寫進 conversation log。Discord transcript 保護一致。

> ⚠️ 注意：MCP server 仍會在 page interaction 內看到注入的值。logging 層面要確認 perch 不把 MCP tool args 全文落地（已知 perch 對 MCP tool call 有截斷，需確認）。

### 4. storageState 雙環境流程

某些網站首次登入需要複雜互動（Google OAuth、生物辨識、SMS OTP），container 內處理太麻煩。**移到 Mac 端做一次**：

```bash
# Mac
tests/playwright-login.sh google
# → headed Chrome 開起來，Doro 手動完成登入
# → 儲存 storageState.json
# → 自動 scp 到 home-auto:/data/playwright/state/google.json
```

Container 內的 Playwright 用 `--storage-state=/data/playwright/state/google.json` 跑 headless，免再登入。

不是每個網站都要走這條路，**簡單表單登入（如富邦帳單頁）直接 container 內 headless + 注入即可**。

### 5. Bot Detection Fallback Ladder

```
Level 0: --headless                              ← 預設
Level 1: + --user-data-dir 養 profile (時間累積)  ← 自動，不需動作
Level 2: + 加 stealth args（disable-blink-features=AutomationControlled）
Level 3: 退到 Xvfb + headed（image +50MB）
Level 4: 該網站 host 端跑（chrome-cdp on Mac），結果 sync 回 container
```

Spec 不一上來實作所有 Level，**只實作 Level 0 + 1**。實際遇到擋再爬樓梯。

## 失敗模式與處理

| 失敗 | 偵測 | 處理 |
|---|---|---|
| CAPTCHA 解析錯 | 表單回 error / 頁面停在登入頁 | Claude 點「重新產生驗證碼」重試（cap 5 次）；連續 3 次失敗截圖求助 Doro |
| 網頁結構變更 | element 找不到 | Claude 截圖 + accessibility tree 描述狀況給 Doro |
| Bot detection (CF challenge / hCaptcha) | 出現非預期頁面 | Claude 截圖警告，建議該站走 Mac 端 storageState 路線 |
| 下載超時 | `download` event 沒在 N 秒內觸發 | Claude 截圖檢查 + 重試一次 |
| Secrets file 缺失 | jq 讀檔失敗 | Skill 明確錯誤訊息，引導 Doro 補檔 |

## 與既有元件的關係

- **不取代** `tests/chrome-agent.sh` + chrome-cdp skill：host (Mac) 端互動 debug、共用 Doro 個人 Chrome session 仍用此方案
- **不衝突** with codex / opencode runtimes：Playwright MCP 透過 Claude config 啟用，僅 Claude runtime 看得到（其他 runtime 不需 browser 能力）
- **可結合** `local-schedule` skill：未來 cron 化「每月初自動抓帳單」走 schedule 觸發 Claude，Claude 用 browser-automation skill 完成

## 不做的事（YAGNI）

- 不裝 Firefox / WebKit（只 Chromium）
- 不裝 noVNC / VNC server
- 不寫 Web UI 看 browser 即時狀態（Discord 截圖足夠）
- 不做多 Claude session 共享 browser 狀態（每個 conv-id 獨立 profile，避免互相污染）
- 不做 OCR fallback（Tesseract）— Claude vision 已經夠用
- 不在 Dockerfile 預先 seed storageState — 那是 runtime concern

## Open Questions

1. `--user-data-dir` 在多個 Claude session 並行時的 lock 行為？需測試（Chromium 預設 single-instance per profile）→ **預期解法**：每個 conv-id 獨立 profile dir
2. perch 的 logging 對 MCP tool args 是否落地全文？需 audit（影響 secrets 注入是否真的安全）
3. Playwright headless Chrome 啟動時間在 Ubuntu 24.04 + 容器資源下能否 < 2s？影響使用體感
