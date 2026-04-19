## ADDED Requirements

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
