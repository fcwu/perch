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

   每個 test case 格式：

   ````markdown
   ## T<N> — <功能描述>

   **層級**：Unit | Integration | E2E-curl | E2E-browser | E2E-gitlab

   **目的**：<一句話說明這個 test 驗證什麼>

   **背景**（若需要）：<前置條件或相關背景知識>

   **步驟**：
   ```bash
   # 具體指令或操作步驟
   ```

   **預期**：
   - <預期結果 1>
   - <預期結果 2>

   **反向驗證**（若適用）：
   ```bash
   # 驗證錯誤情況
   ```

   **自動化**（若有對應 unit test）：`go test -run TestXxx ./...`
   ````

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

6. **Append 到目標測試檔末尾**

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
