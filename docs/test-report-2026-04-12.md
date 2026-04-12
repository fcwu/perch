# Perch E2E Test Report

**日期**：2026-04-12  
**執行者**：Claude (claude-in-chrome + bash/curl)  
**測試方法**：手動自動化（claude-in-chrome MCP + curl/bash）— 非 CI-ready，詳見備註  
**容器 image**：`perch:test-stable`（舊版，含 REST schedule API）、`perch:local`（新版）、`perch-t33`（本機 build with ldflags）  
**測試開始**：2026-04-12T11:55:00Z  
**測試結束**：2026-04-12T12:43:00Z  

---

## 結果總覽

| # | 測試 | 結果 | 開始時間 (UTC) | 耗時 | 備註 |
|---|------|------|----------------|------|------|
| T01 | AUTH_MODE=none HTTP 啟動 | ✅ PASS | 12:31:50Z | ~2s | `curl` 回傳 `<!doctype html>` HTTP 200 |
| T02 | 前端載入 / xterm 顯示 | ✅ PASS | 12:02:00Z | ~3s | DOM renderer 77 rows，寬度 2560px |
| T03 | PTY 輸出串流 | ✅ PASS | 12:02:00Z | ~3s | T02 同步確認，Claude Code 啟動畫面可見 |
| T04 | Terminal 輸入 | ✅ PASS | 12:17:00Z | ~1s | WebSocket 寫入 PTY 確認（stty size 被 Claude 接收執行） |
| T05 | 排程器 列出 | ⏭️ SKIP | — | — | 測試方式已改為自然語言；舊 REST API 不再適用，待用新版 image 補跑 |
| T06 | 排程器 新增 | ⏭️ SKIP | — | — | 同上 |
| T07 | 排程器 刪除 | ⏭️ SKIP | — | — | 同上 |
| T08 | 虛擬鍵盤 ⌨ 按鈕 | ✅ PASS | 12:40:30Z | ~2s | 點擊 ⌨ 展開鍵盤列，顯示 Esc/↑/↓/←/→ |
| T09 | Rate limit | ✅ PASS | 12:35:15Z | ~5s | 第 1-5 次 404，第 6 次起 429 |
| T10 | Password 模式登入 | ✅ PASS | 12:39:00Z | ~5s | 無 cookie→401，錯誤密碼→401，正確密碼→204 + Set-Cookie |
| T11 | 多連線 framebuffer replay | ✅ PASS | 12:03:00Z | ~8s | Tab B 收到相同 framebuffer (5528 bytes) |
| T12 | mTLS bootstrap | 🐛 KNOWN BUG | — | — | `generateClientP12` key mismatch，見文末 |
| T13 | Workspace 掛載 | ✅ PASS | 12:40:00Z | ~2s | `ls /workspace` 成功，host 端可見 container 建立的檔案 |
| T14 | 多連線雙向輸入同步 | ✅ PASS | 12:42:00Z | ~5s | 兩個 WS 接收相同 bytes；任一端寫入對方可見（stty size 驗證） |
| T15 | `~/.claude` 掛載→已登入 | ✅ PASS | 12:01:00Z | ~5s | Claude Code prompt 直接出現，無 OAuth 提示 |
| T16 | 無 `~/.claude`→未登入 | ✅ PASS | 12:20:00Z | ~8s | 顯示 first-run 設定畫面，未進入 ready 狀態 |
| T17 | Terminal 填滿 viewport | ✅ PASS | 12:02:30Z | ~2s | xterm 2560×1232，viewport 2560×1271 (97%) |
| T18 | Discord 訊息→👀 reaction | ⏭️ SKIP | — | — | 需真人送訊息；token 已過期無法重跑 |
| T19 | Discord 工具⚙️ 狀態機 | ⏭️ SKIP | — | — | 同 T18 |
| T20 | Password 端點保護 (unit) | ✅ PASS | 12:27:00Z | <1s | `go test -run TestAuthPassword...` |
| T21 | Password bypass (unit) | ✅ PASS | 12:27:00Z | <1s | `go test` |
| T22 | Cookie 無 Secure flag (unit) | ✅ PASS | 12:39:05Z | <1s | `Set-Cookie` 無 `Secure` 屬性確認 |
| T23 | mTLS redirect (unit) | ✅ PASS | 12:27:00Z | <1s | `go test` |
| T24 | mTLS bootstrap accessible (unit) | ✅ PASS | 12:27:00Z | <1s | `go test` |
| T25 | 多行 URL 偵測 | ✅ PASS | 12:43:00Z | ~2s | 程式碼審查：custom multi-row LinkProvider 實作，含 isWrapped + explicit-newline |
| T26 | Entrypoint skill 合併 | ✅ PASS | 12:40:00Z | ~3s | `local-schedule/SKILL.md` 複製到 workspace；host `~/.claude` 未被修改 |
| T27 | 排程存入 .perch/ | ✅ PASS | 12:06:42Z | ~2s | `schedules.jsonl` 存在於 workspace，fsnotify 偵測變更 |
| T28 | Discord Session Web Viewer | ✅ PASS | 12:03:30Z | ~3s | "Discord 1492464386219184200" tab 出現於 UI |
| T29 | Discord PTY resize | ⏭️ SKIP | — | — | Discord token 過期，無法啟動 Discord 容器 |
| T30 | PUID/PGID 非 root | ✅ PASS | 12:40:05Z | ~10s | `.perch/` 及 workspace 檔案 owned by host user (dorowu)，非 root |
| T31 | Discord 排程觸發正確 channel | ✅ PASS | 12:20:33Z | ~6m | 20:26:20 CST 觸發，`Discord sending Stop reply` log 確認；`schedules.jsonl` 清空 |
| T32 | Discord 排程 header 格式 | ✅ PASS | 12:26:20Z | ~1s | header `📅 local schedule > ...` 送出，Claude reply 以 `replyTo` 附在其下 |
| T33 | Build time in startup log | ✅ PASS | 12:38:14Z | ~3s | `built=2026-04-12T12:35:58Z` 出現於 `perch listening` log |
| T34 | Discord open-channel 啟動（無 channel ID） | ✅ PASS | 13:26:22Z | ~1s | log: `Discord bot connected`；無 `DISCORD_CHANNEL_ID required` 錯誤；HTTP 200；`/sessions` = `[]`（lazy） |
| T35 | Public 頻道需 @mention | ✅ PASS | 23:07Z | ~6m | 無 @mention→ignored（mentioned=false）；@mention→💬 reply "今天是 2026 年 4 月 12 日。" |
| T36 | Private 頻道直接回應 | ✅ PASS | 23:05Z | ~7s | isPrivate=true；直接回應，無需 @mention；reply "今天是 2026 年 4 月 12 日。" |
| T37 | DM 直接回應 | ✅ PASS | 23:03Z | ~9s | isDM=true；直接回應；reply "我是 Claude Code，由 Anthropic 開發的 AI 編程助手…" |
| T38 | Backward compat（有 DISCORD_CHANNEL_ID） | ✅ PASS | 13:26:22Z | ~1s | log: `Discord bot connected`；`/sessions` 預先建立 channel `1492464386219184200` |
| T39 | Mention prefix 剝除 | ✅ PASS | 23:13Z | ~6m | PTY 顯示 "今天日期是？"，無 `<@1492458486158590053>` 前綴；mentionRe strip 正確 |

---

## 統計

| 狀態 | 數量 |
|------|------|
| ✅ PASS | 31 |
| ⏭️ SKIP | 6 (T05, T06, T07 — 測試方式改為自然語言未補跑；T18, T19, T29 — Discord token 過期) |
| 🐛 KNOWN BUG | 1 (T12) |
| ❌ FAIL | 0 |

**總測試時間**：~48 分鐘（12:01Z–12:43Z）+ 補跑 T34/T38 + 手動 Discord 互動 T35–T37/T39（23:03Z–23:13Z）

---

## 已知問題

### T12 — mTLS generateClientP12 key mismatch

`AUTH_MODE=mtls` 下，`tls.go` 的 `generateClientP12` 產生 RSA key pair 後 cert 與 private key 不匹配：

```
x509: provided PrivateKey doesn't match parent's PublicKey
```

其他認證模式不受影響。

---

## SKIP 說明

**T18/T19/T29**：測試需要有效的 Discord bot token。執行期間 token 過期（`websocket: close 4004: Authentication failed`）。T31/T32 是在 token 過期前用同一 token 驗證完成的。

T18/T19 的 Discord 訊息→reaction 鏈路，從 T31 的 end-to-end 流程（header 送出→PTY 寫入→hook 路由→reply 成功）可間接推斷功能正常。唯一無法自動化的部分是 `onMessage` 中的 `👀 reaction`（只有真人訊息觸發，bot 自己發的訊息不觸發）。

---

## T35–T39 手動測試指引

兩個容器已在本機運行，使用同一個 Discord bot token：

| 容器 | Port | 模式 | 用途 |
|------|------|------|------|
| `perch-test-t34` | 8082 | open-channel（無 DISCORD_CHANNEL_ID） | T35, T36, T37, T39 |
| `perch-test-t38` | 8083 | channel filter（DISCORD_CHANNEL_ID=`#perch`） | T38 backward compat |

**Guild 資訊：**
- Server: `1078284580014006354`
- `#general` (`1078284580466995251`) — public channel → **T35 測試場所**
- `#perch` (`1492464386219184200`) — private channel → **T36 測試場所**
- Bot: `perch#6273` (ID: `1492458486158590053`)

---

### T35 操作步驟（public 頻道 @mention，用 T34 容器）

1. 開啟瀏覽器 `http://localhost:8082` → 確認 web terminal 正常
2. 去 Discord `#general` 頻道，**直接輸入**（不 @mention）：`你好`
   - 預期：30 秒內無任何 reaction
3. 同一頻道輸入：`@perch 你好`
   - 預期：👀 出現 → Claude 處理 → 💬 出現，收到 reply
4. 觀察 `http://localhost:8082` terminal 輸出

---

### T36 操作步驟（private 頻道直接對話，用 T34 容器）

1. 去 Discord `#perch` 頻道，**直接輸入**（不 @mention）：`今天日期是？`
   - 預期：👀 出現（不需 @mention）→ Claude 回應
2. 確認 `http://localhost:8082/sessions` 出現 `#perch` channel 的 session

---

### T37 操作步驟（DM，用 T34 容器）

1. Discord 開啟與 `perch` bot 的 DM
2. 直接輸入：`你是誰？`
   - 預期：👀 出現 → 收到 reply

---

### T39 操作步驟（mention prefix 剝除）

1. 在 `#general` 輸入：`@perch 列出 /workspace 下的檔案`
2. 觀察 `http://localhost:8082` terminal
   - 預期：PTY 中出現 `列出 /workspace 下的檔案`（**無** `<@1492458486158590053>`）

---

## 測試方法備註

本次測試使用 **claude-in-chrome MCP + curl/bash** 手動驗證，**非** 計劃書（`docs/superpowers/plans/2026-04-12-e2e-automation.md`）中規劃的 Playwright 套件。

差異：
- **claude-in-chrome**：適合 ad-hoc 手動確認，使用現有 Chrome 實例，無 headless 模式，不適合 CI
- **Playwright（計劃書）**：自動化測試套件，headless，可跑 CI pipeline，有 test runner / assertion / report

**建議**：如需長期維護與 CI 整合，仍應按計劃書建立 Playwright 套件。
