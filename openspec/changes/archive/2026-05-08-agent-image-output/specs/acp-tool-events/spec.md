## ADDED Requirements

### Requirement: RunCompleted 回應包含圖片附件記錄

當 Perch 處理 `RunCompleted` ACP 事件時，應從文字中提取 `[image: ...]` token、解析為已儲存圖片檔案，並將產生的附件記錄包含在轉發給所有下游用戶端的結構化回應中。

#### Scenario: 圖片記錄包含於 SSE message 事件

- **WHEN** Perch 處理 `RunCompleted` 並提取到一個以上的圖片附件
- **THEN** 傳送給 Web chat 用戶端的 SSE `message` 事件酬載應包含 `images` 欄位：`{url: string, caption: string}` 物件陣列

#### Scenario: 無圖片 token 時 images 欄位為空

- **WHEN** `RunCompleted` 文字不含任何 `[image: ...]` token
- **THEN** SSE `message` 事件酬載應包含 `"images": []`，或省略該欄位（用戶端將缺少視為空陣列）

#### Scenario: 圖片提取不影響 RunCompleted 既有語意

- **WHEN** 從 `RunCompleted` 文字提取圖片 token
- **THEN** 所有既有的 `RunCompleted` 行為（ManagementHub 淘汰、`query_sessions` 最終化、`tool_events` 最終化）維持不變
