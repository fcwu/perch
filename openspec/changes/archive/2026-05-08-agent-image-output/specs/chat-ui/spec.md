## ADDED Requirements

### Requirement: 助理訊息氣泡內嵌渲染圖片

當 Chat API 回應包含圖片附件時，Web chat 訊息氣泡應在文字內容下方依序渲染每張圖片。

#### Scenario: 單張圖片渲染於文字下方

- **WHEN** SSE `message` 事件包含 `{"text": "...", "images": [{"url": "/api/images/...", "caption": "screenshot.png"}]}`
- **THEN** 訊息氣泡顯示 markdown 渲染後的文字，其下方跟著一個 `<img>` 元素，`src` 設為 URL，`alt` 設為 caption

#### Scenario: 多張圖片依序垂直排列

- **WHEN** 回應包含兩個以上圖片附件記錄
- **THEN** 每張圖片依 `images` 陣列順序垂直堆疊顯示於文字下方

#### Scenario: 圖片載入失敗顯示佔位符

- **WHEN** 瀏覽器無法從 `/api/images/...` URL 載入圖片
- **THEN** `<img>` 元素顯示破圖佔位符，並以 caption 文字作為備用說明

#### Scenario: 無圖片時僅顯示文字

- **WHEN** 回應的 `images` 欄位為空陣列或不存在
- **THEN** 訊息氣泡渲染方式與原本相同（純文字，無額外 DOM 元素）
