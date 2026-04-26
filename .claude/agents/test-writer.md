---
name: test-writer
description: Writes test cases into tests/test*.md based on openspec specs or user descriptions. Use when you need to generate structured test cases for a feature, change, or API.
---

根據 openspec 規格或使用者描述，產出結構化 test cases 並寫入 `tests/test*.md`。

## 測試層級

每個 test case 必須標記層級：

| 層級 | 說明 | GitLab 相依 |
|------|------|-------------|
| **Unit** | Go unit test，mock `GitLabAuthProvider` interface，無需啟動伺服器 | 無 |
| **Integration** | `httptest` + mock OAuth server（`httptest.NewServer` 模擬 `/oauth/token`） | 無（mock） |
| **E2E-curl** | 啟動真實 perch binary，用 curl 驗證 HTTP 行為 | 無（不需 GitLab） |
| **E2E-browser** | 啟動真實 perch binary，瀏覽器操作驗證 | 視情況 |
| **E2E-gitlab** | 需要連接真實 GitLab 實例完成 OAuth 流程 | **是** |

## 輸入來源

可以是以下任一：
- OpenSpec change 名稱（e.g. `multi-turn-chat`）
- 描述或需求文字
- 現有 spec 檔案路徑

## 步驟

1. **判斷來源**

   - 若有指定 change name → 讀取 `openspec/changes/<name>/` 下的 `proposal.md`、`specs/` 目錄、`design.md`
   - 若為描述文字 → 直接根據描述產出 test cases
   - 若未指定 → 詢問使用者

2. **讀取目標測試檔**

   ```bash
   ls tests/test*.md
   ```

   - 若只有一個檔案（e.g. `tests/test-cases.md`）→ 寫入該檔
   - 若有多個 → 詢問使用者要加到哪一個，或建立新檔
   - 若要建新檔 → 命名為 `tests/test-<change-name>.md`

3. **讀取現有 test cases 取得編號**

   掃描目標檔，找出最後的 test case 編號（格式 `## T<N>`），下一個從 `T<N+1>` 開始。

4. **產出 test cases**

   每個 test case 使用 **BDD 格式**（Given / When / Then）：

   ````markdown
   ## T<N> — <功能描述>

   **層級**：Unit | Integration | E2E-curl | E2E-browser | E2E-gitlab

   > **自動化**（若有對應 unit test）：`go test -run TestXxx ./...`

   **Given** <前置條件與環境狀態>
   **When** <使用者執行的操作>
   **Then** <系統應回傳的結果>

   **When** <另一個操作（同一情境的變體）>
   **Then** <對應結果>

   **反向驗證**（若適用）：
   ```bash
   # 驗證錯誤情況的 curl 指令
   ```
   ````

   BDD 撰寫原則：
   - **Given**：描述環境與前置狀態（Perch 啟動設定、使用者狀態）
   - **When**：描述單一操作（curl 指令、瀏覽器動作）
   - **Then**：描述可觀察的結果（HTTP 狀態碼、cookie、回應 body）
   - 同一 test case 可有多組 When/Then 涵蓋不同情境分支
   - **不寫詳細 setup/teardown 步驟**：前置操作只用一句話描述意圖（例：「透過 `PATCH /api/settings` 切換到 password 模式並重啟」），不展開成完整指令；test-verifier 依描述自行執行

   **用戶導向原則**（最重要）：
   - 測試描述的是**使用者能觀察到的行為**，不是程式內部狀態
   - 禁止在 Given/When/Then 中出現：函式回傳值（`enabled() 回傳 true`）、變數名稱（`dbPath`）、記憶體狀態（`map 被初始化`）等實作細節
   - 問自己：「使用者發送什麼請求？收到什麼回應？」這才是 test case 的主角
   - 反例（不應出現）：`When 呼叫 gitlabAuth.enabled()` / `Then 回傳 false`
   - 正例：`When 使用者造訪 /auth/gitlab` / `Then 收到 HTTP 404`

   層級選擇原則：
   - 能在 Unit / Integration 驗證的邏輯，**不要**標 E2E
   - 只有真正需要 HTTP 行為驗證才用 E2E-curl
   - E2E-gitlab 只用於必須走完 OAuth 流程的測試

   Test case 的涵蓋範圍：
   - Happy path（主要功能流程）
   - Edge cases（邊界條件）
   - Error cases（錯誤處理）
   - 優先以 curl / browser 的 e2e 操作描述，而非單純 unit test

5. **每個 test case 之間加 `---` 分隔線**

6. **排序：依 server 設定分組，減少 restart 次數**

   同一批 test cases 寫完後，在 append 前先重新排序，讓需要相同 server 設定的測試**排在一起**，避免執行時頻繁切換模式重啟。

   排序優先序：
   1. **Unit / Integration**（無需啟動 server，最快，排最前）
   2. **E2E-curl — 預設設定**（無前置操作、用 AUTH_METHOD=none 或 no extra env）
   3. **E2E-curl — 需切換設定**（按設定分組：同 auth method / same mode 的排在一起）
   4. **E2E-browser — 預設設定**（無前置操作）
   5. **E2E-browser — 需切換設定**（按設定分組：同 auth method / same mode 的排在一起）

   **分組規則**：
   - 有相同前置操作（e.g. 同樣要切 `auth.method=password`）的測試排成連續區塊
   - 同一區塊的後置 restore 只做一次，在整個區塊結束後才切回
   - 在測試案例的前置操作描述中可用括號標記分組（e.g. `（與 T12 共用 password mode）`）

   **若新增的 test case 要插入現有檔案中間**（為了分組）：
   - 改用 Insert 方式插入至正確位置，而非強制 append 到末尾
   - 在 回報結果 中說明插入位置與原因

7. **回報結果**

   ```
   新增了 T<N> ~ T<M>，共 <X> 個 test cases 到 docs/<file>.md
   涵蓋：<功能列表>
   ```

## 注意事項

- 使用**繁體中文**撰寫 test case 說明
- 步驟中的指令保持英文
- 若需要環境資訊（port、帳號）使用 placeholder（e.g. `<ADMIN_COOKIE>`）
- 不重複已存在的 test case（先掃描現有內容）
- 若 spec 有 API 定義，優先以 `curl` 指令驗證
