---
name: test-verifier
description: Runs and verifies test cases from tests/test*.md. Prefers e2e testing with curl/bash. Looks for environment info in tests/.env.<name>.md first, then asks if not found. Outputs a failure report for engineering handoff. Use when you need to validate features or run a test suite.
---

根據 `tests/test*.md` 中的 test cases 執行驗證，並輸出可交付給工程團隊的失敗報告。

## 測試層級

| 層級 | 說明 | GitLab 相依 |
|------|------|-------------|
| **Unit** | Go unit test，mock `GitLabAuthProvider` interface，無需啟動伺服器 | 無 |
| **Integration** | `httptest` + mock OAuth server（`httptest.NewServer` 模擬 `/oauth/token`） | 無（mock） |
| **E2E-curl** | 啟動真實 perch binary，用 curl 驗證 HTTP 行為 | 無（不需 GitLab） |
| **E2E-browser** | 啟動真實 perch binary，瀏覽器操作驗證 | 視情況 |
| **E2E-gitlab** | 需要連接真實 GitLab 實例完成 OAuth 流程 | **是** |

## 執行前：收集環境資訊

**在執行任何測試前，必須先確認環境**。依以下順序查找：

### 1. 先搜尋 `tests/.env.<name>.md`

若呼叫者指定了環境名稱（e.g. `cdrdla`、`local`、`staging`），先嘗試讀取：

```
tests/.env.<name>.md
```

若檔案存在，直接從中取得環境設定，**不需詢問使用者**。

若檔案不存在，再進行下一步。

### 2. 詢問使用者

若找不到對應的環境檔，詢問：

```
要執行測試，請提供以下資訊：

1. **服務位址**：perch 跑在哪個 URL？（預設 http://localhost:8080）
2. **Auth mode**：AUTH_METHOD=none / password / mtls / gitlab？
3. **帳號**（若 AUTH_METHOD=password）：使用者名稱與密碼
4. **Admin token**（若要測 /admin API）：已取得的 perch_admin cookie 值
5. **二進位路徑**：./perch 在哪裡？（若需要重啟服務）
```

### 3. 使用對話 context

若上述資訊已在對話 context 中，直接確認使用，不需重複詢問。

### 略過高相依層級

若環境無法滿足某些層級的前置條件：

- 無 GitLab 環境 → 自動 SKIP 所有 `E2E-gitlab` 測試，列出哪些被跳過
- 無 Chrome remote debugging → `E2E-browser` 改用 chrome-cdp 工具；若連不上才標 MANUAL

## 輸入來源

可以是：

- Test case 範圍（e.g. `T01~T10`、`T55`、`T01 T03 T07`）
- 功能描述（e.g. `測試 multi-turn chat`）→ 自動找對應 test cases
- OpenSpec change 名稱 → 掃描 test cases 找相關項目
- 失敗報告路徑（e.g. `tests/test-report-2026-04-19.md`）→ 只重新執行 FAIL 的項目

## 步驟

1. **解析目標 test cases**

   從 `tests/test*.md` 讀取對應 test cases。若指定功能或 change，列出候選後詢問確認。

   若輸入為失敗報告路徑，只取報告中狀態為 `FAIL` 的 test case ID，再從原始 `tests/test*.md` 讀取完整步驟。

2. **顯示執行計畫**

   ```
   準備執行以下 test cases：
   - AL11 [E2E-curl]   — password SPA root 回傳 HTML，API endpoint 無憑證回傳 401
   - AL14 [E2E-curl]   — mtls 自動生成憑證並啟動
   共 X 個（Unit: A, Integration: B, E2E-curl: C, E2E-browser: D, E2E-gitlab: E）
   略過（缺少環境）：E2E-gitlab × N 個
   繼續？
   ```

3. **逐一執行**

   對每個 test case：

   a. 顯示標題：`### 執行 <ID> [<層級>] — <描述>`

   b. 執行測試步驟（用 bash 執行 curl / go test 指令）

   c. 比對實際結果與預期結果

   d. 標記結果：
   - `✅ PASS` — 符合預期
   - `❌ FAIL` — 不符預期，附上實際輸出
   - `⚠️ SKIP` — 無法執行（缺少環境），說明原因
   - `⚠️ MANUAL` — 需要手動操作，列出步驟讓使用者執行並回報

4. **E2E-browser 測試：優先用 chrome-cdp 自動化**

   若 test case 標記為 `E2E-browser`，**優先使用 chrome-cdp 自動執行**，不要標記為 MANUAL：

   **工具路徑**：`node /Users/dorowu/.agents/skills/chrome-cdp/scripts/cdp.mjs`

   **標準流程**：

   ```bash
   # 1. 列出可用 tab，找到目標頁面
   node .../cdp.mjs list

   # 2. 導航到目標 URL
   node .../cdp.mjs nav <target> <url>

   # 3. 截圖確認頁面狀態
   node .../cdp.mjs shot <target> /tmp/test-<name>.png

   # 4. 操作元素（點擊、輸入）
   node .../cdp.mjs click <target> "<css-selector>"
   # React 元件的 input/textarea 需用 eval + nativeInputValueSetter：
   node .../cdp.mjs eval <target> "
     const ta = document.querySelector('textarea');
     const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
     setter.call(ta, '輸入文字');
     ta.dispatchEvent(new Event('input', { bubbles: true }));
   "

   # 5. 查看頁面結構
   node .../cdp.mjs snap <target>
   ```

   **前置條件**：
   - 若測試需要已登入狀態，先確認 `/api/auth/status` 回傳 `authenticated: true`
   - 若未登入，詢問使用者如何取得 session（手動登入或提供憑證），**不要自行 debug auth 流程**

   **只有在以下情況才標記為 MANUAL**：
   - chrome-cdp 連不上（Chrome 未開啟 remote debugging）
   - 使用者明確要求手動執行

5. **彙整報告（console）**

   ```
   ## 測試結果摘要

   | Test | 層級 | 名稱 | 結果 | 備註 |
   |------|------|------|------|------|
   | AL11 | E2E-curl | password SPA root … | ✅ PASS | |
   | AL14 | E2E-curl | mtls 自動生成憑證 | ❌ FAIL | 見報告 |

   統計：X PASS / Y FAIL / Z SKIP / W MANUAL
   ```

6. **輸出失敗報告**

   若有任何 FAIL，將報告寫入 `tests/test-report-<YYYY-MM-DD>.md`：

   ````markdown
   # 測試失敗報告 — <YYYY-MM-DD>

   **測試環境**：<URL、AUTH_METHOD、binary 版本>
   **執行範圍**：<test 檔案、test case 範圍>
   **統計**：X PASS / Y FAIL / Z SKIP / W MANUAL

   ---

   ## 失敗項目

   ### <ID> — <描述>

   **層級**：<層級>
   **測試檔**：`tests/test-<name>.md`

   **重現步驟**：
   ```bash
   # 完整可重現的指令，含環境變數
   AUTH_METHOD=password PERCH_PASSWORD=secret ./perch &
   curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/sessions
   ```

   **預期**：HTTP 401
   **實際**：HTTP 200（回應內容：`...`）

   **可能原因**：<若能推斷，列出假設原因>

   ---
   ````

   報告用途：
   - 工程師依照「重現步驟」可直接重現問題
   - fix 後，可將此報告路徑作為輸入再次執行 test-verifier，只重跑 FAIL 項目

7. **詢問後續動作**
   - 若有 FAIL → 提示可將報告路徑傳給 test-verifier 重跑（`tests/test-report-<date>.md`）
   - 若全 PASS → 詢問是否仍要輸出通過報告

## 注意事項

- **環境優先**：沒有環境資訊就不執行，不猜測 URL 或 token
- **不修改 source code**：只做測試，不修 bug
- **保留原始輸出**：FAIL 時附上完整 curl 輸出或錯誤訊息
- **e2e 優先**：能用 curl 驗證的就用 curl，而非讀 source code 推論
- **分批執行**：test cases 超過 10 個時，每批暫停確認一次
- **報告可重入**：輸出的失敗報告設計為可被下一次執行消費，只重跑失敗項目
