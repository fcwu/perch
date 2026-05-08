## Requirements
### Requirement: Chat input and streaming response

The `/chat` page SHALL provide a text input for the user to submit a query, and display the OpenCode response as it streams in, rendered as markdown.

#### Scenario: user submits a query
- **WHEN** a logged-in user types a question and presses Enter or clicks Send
- **THEN** the frontend SHALL POST `{"query": "<text>"}` to `/api/chat`, then open `/ws/chat?id=<userID>` to receive streaming output

#### Scenario: response streams in
- **WHEN** PTY output bytes arrive over the WebSocket
- **THEN** the frontend SHALL progressively render the accumulated text as markdown in the response area

#### Scenario: session in progress indicator
- **WHEN** the OpenCode session is running
- **THEN** the frontend SHALL show a loading indicator and disable the input field until the Stop event is received

### Requirement: Tool call expand panel

A collapsible side panel SHALL display real-time tool call events during an active session.

#### Scenario: tool starts
- **WHEN** a `tool_start` JSON event arrives on the WebSocket (separate from raw PTY bytes)
- **THEN** the panel SHALL add a new entry showing the tool name and truncated input, with a spinner

#### Scenario: tool completes
- **WHEN** a `tool_end` JSON event arrives for a previously started tool
- **THEN** the panel SHALL update that entry: replace the spinner with ✓, show elapsed time

#### Scenario: panel collapsed by default
- **WHEN** the page loads
- **THEN** the tool call panel SHALL be collapsed; users can expand it by clicking a toggle button

### Requirement: GitLab login gate

The `/chat` page SHALL redirect unauthenticated users to `/auth/gitlab`.

#### Scenario: unauthenticated access
- **WHEN** a user navigates to `/chat` without a valid session cookie
- **THEN** the server SHALL redirect to `/auth/gitlab`

### Requirement: Conversation list with pinned group and Load More

The `/chat` page SHALL display the user's conversations in a left-rail list with two sections: "Pinned" (above) and "Recent" (below). The list SHALL fetch the first page from `GET /api/conversations`. When the user scrolls to the bottom of the Recent section, a "Load More" button SHALL appear and trigger `GET /api/conversations?before=<cursor>&limit=20`.

#### Scenario: First load shows pinned + recent

- **WHEN** the user visits the chat page
- **THEN** the list shows pinned rows on top (ordered by `pinned_at` DESC) and recent rows below (ordered by `updated_at` DESC)

#### Scenario: Load More appends older conversations

- **WHEN** the user clicks "Load More"
- **THEN** the next batch of conversations is appended below the current Recent list, and pinned rows are NOT duplicated

#### Scenario: End-of-list hides Load More

- **WHEN** the API response indicates no further pages (`next_before` is null/0 or fewer than `limit` rows returned)
- **THEN** the "Load More" button is hidden

### Requirement: Pin / unpin button per conversation row

Each conversation row in the list SHALL show a pin icon. Clicking it SHALL call `PATCH /api/conversations/{id}` to toggle the `pinned` flag, and the UI SHALL move the row between Pinned and Recent without a full page reload.

#### Scenario: Pinning a row moves it to the Pinned section

- **WHEN** the user clicks the pin icon on a Recent-section row
- **THEN** the UI moves the row into the Pinned section at the top and its icon switches to the "pinned" state

#### Scenario: Unpinning moves it back to Recent

- **WHEN** the user clicks the icon on a Pinned-section row
- **THEN** the row returns to its position in the Recent section sorted by `updated_at`

### Requirement: Runtime + model dropdown in the conversation header

The chat header SHALL show the active conversation's runtime and model and provide a dropdown to switch them. Selecting a different runtime/model combination SHALL call `PATCH /api/conversations/{id}` and SHALL display a confirmation dialog warning that switching starts a fresh agent context.

#### Scenario: Header shows current runtime+model

- **WHEN** the user opens a conversation
- **THEN** the header displays a control labelled with the conversation's current runtime+model (e.g., "Claude · Sonnet 4.6")

#### Scenario: Switching warns about context reset

- **WHEN** the user picks a different runtime+model from the dropdown
- **THEN** a confirmation dialog appears with text indicating the new agent will not see prior runtime memory
- **AND** confirming sends `PATCH /api/conversations/{id}` with the new fields

#### Scenario: New-conversation form remembers the user's preferred runtime+model

- **WHEN** the user starts a new conversation
- **THEN** the new-conversation dialog defaults to the user's preferred runtime+model (or server default if not set)

### Requirement: Schedule button on chat header opens a CRUD panel

The chat header SHALL include a clock-icon button that opens a side panel listing the conversation's schedules (`GET /api/conversations/{id}/schedules`) with controls to create a new schedule (`POST`) and to cancel an existing schedule (`DELETE`).

#### Scenario: Open schedule panel

- **WHEN** the user clicks the clock icon
- **THEN** a side panel opens showing existing schedules with their time, prompt preview, and a delete button each

#### Scenario: Create a new daily schedule

- **WHEN** the user fills the form with hour, minute, repeat, prompt and submits
- **THEN** the panel calls `POST /api/conversations/{id}/schedules` and the new row appears in the list

#### Scenario: Create a new one-shot schedule

- **WHEN** the user picks a specific datetime instead of daily
- **THEN** the panel sends `one_shot_at` as epoch-ms and the row is shown with a "one-shot" indicator

### Requirement: Scheduled-fire turns are visually marked

The chat thread SHALL render a small clock icon (or equivalent badge) next to any turn whose `source` field equals `'schedule'` so users can distinguish manually-typed turns from scheduler-fired turns.

#### Scenario: Scheduled turn shows clock badge

- **WHEN** the message-list endpoint returns a turn with `source='schedule'`
- **THEN** the rendered turn includes a clock badge tooltip "Triggered by schedule"

### Requirement: Delete conversation affordance

The conversation list SHALL expose a delete option per row (e.g., on a context menu or a hover-revealed trash icon). Clicking it SHALL prompt for confirmation and then call `DELETE /api/conversations/{id}`. On success, the row is removed from the list and, if it was the active conversation, the chat pane resets to a "New conversation" state.

#### Scenario: Delete from list

- **WHEN** the user invokes delete on a row and confirms
- **THEN** the UI calls `DELETE /api/conversations/{id}`, removes the row, and any schedule rows for that conversation disappear from the schedule panel as well (cascade)

#### Scenario: Deleting the active conversation resets the pane

- **WHEN** the user deletes the conversation currently open in the chat pane
- **THEN** the pane clears and presents the "New conversation" experience

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

