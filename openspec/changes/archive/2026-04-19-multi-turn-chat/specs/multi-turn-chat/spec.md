## ADDED Requirements

### Requirement: Server reconstructs per-user conversation history from query_sessions
The system SHALL query the `query_sessions` table for the authenticated user's completed (`status='done'`) sessions within the last 24 hours, ordered oldest-first, to build conversation history. No new table or in-memory store is required.

#### Scenario: Recent sessions are used as conversation history
- **WHEN** an authenticated user submits a query and has completed sessions within the last 24 hours
- **THEN** those sessions' queries and responses are included as prior turns in the agent prompt

#### Scenario: Sessions older than 24 hours are excluded
- **WHEN** a user's most recent session is more than 24 hours old
- **THEN** the agent receives only the raw query with no history prefix

#### Scenario: No prior sessions produces no prefix
- **WHEN** an authenticated user has no completed sessions in the last 24 hours
- **THEN** the agent receives only the raw query with no history prefix

### Requirement: Server prepends conversation history to each agent query
The system SHALL format prior turns as a structured prefix and concatenate it with the new query before passing the combined text to the agent runtime.

#### Scenario: History is injected before the current query
- **WHEN** a user has N prior turns within the 24-hour window and submits a new query Q
- **THEN** the agent receives a prompt with all prior User/Assistant pairs followed by Q

#### Scenario: History is capped at 20 most recent turns
- **WHEN** a user has more than 20 completed sessions in the 24-hour window
- **THEN** only the 20 most recent sessions are included in the history prefix

### Requirement: Client can reset conversation history via new_conversation flag
The POST /api/chat request body SHALL accept an optional boolean field `new_conversation`. When `true`, the server SHALL skip history lookup and invoke the agent with no prior context.

#### Scenario: new_conversation skips history lookup
- **WHEN** the client sends POST /api/chat with `new_conversation: true`
- **THEN** the server does not query prior sessions and the agent receives only the current query

#### Scenario: Omitting new_conversation uses history normally
- **WHEN** the client sends POST /api/chat without the `new_conversation` field
- **THEN** the server queries recent sessions and injects history as normal

### Requirement: Web UI renders all turns in a scrollable thread
The chat frontend SHALL display all turns of the current conversation as a vertically scrolled message thread with alternating user and assistant bubbles.

#### Scenario: Multiple turns are visible
- **WHEN** the user has sent two or more messages in a conversation
- **THEN** all prior question/answer pairs are visible above the most recent exchange

#### Scenario: New assistant response is appended
- **WHEN** a new agent response completes
- **THEN** the response is appended to the thread without replacing earlier turns

### Requirement: User can start a new conversation from the UI
The frontend SHALL provide a "New conversation" control that clears the local message thread and sends `new_conversation: true` with the next query.

#### Scenario: New conversation clears the thread and bypasses server history
- **WHEN** the user activates the "New conversation" control and submits the next query
- **THEN** the local thread is cleared and the server skips history lookup for that query

### Requirement: Discord, Telegram, and scheduler flows are unaffected
The system SHALL NOT inject conversation history into agent invocations triggered by Discord messages, Telegram messages, or the scheduler.

#### Scenario: Discord query has no conversation prefix
- **WHEN** an agent session is started from a Discord message
- **THEN** the query is passed to the agent with no conversation history prefix

#### Scenario: Scheduler query has no conversation prefix
- **WHEN** an agent session is started by the scheduler
- **THEN** the query is passed to the agent with no conversation history prefix
