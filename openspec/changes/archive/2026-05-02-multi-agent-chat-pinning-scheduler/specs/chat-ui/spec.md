## ADDED Requirements

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
