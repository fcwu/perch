## Requirements

### Requirement: Server reconstructs per-user conversation history from query_sessions

The system SHALL query the `query_sessions` table for the authenticated user's completed (`status='done'`) sessions belonging to the **active conversation** (matching `conversation_id`), ordered oldest-first, to build conversation history. There SHALL be no time cutoff: the user's own conversations are retained indefinitely until they delete the conversation. No new table or in-memory store is required beyond the existing `conversations` and `query_sessions`.

#### Scenario: All prior turns in the conversation are used as history

- **WHEN** an authenticated user submits a query in conversation `C` and has completed sessions in `C`
- **THEN** those sessions' queries and responses are included as prior turns in the agent prompt, regardless of how old they are

#### Scenario: A different conversation's turns are not pulled in

- **WHEN** the user has prior turns in conversation `A` and starts a new query in conversation `B`
- **THEN** only `B`'s prior turns are considered for history; `A`'s turns SHALL NOT leak into `B`

#### Scenario: No prior sessions in the conversation produces no prefix

- **WHEN** the active conversation has no completed sessions
- **THEN** the agent receives only the raw query with no history prefix

### Requirement: Server prepends conversation history to each agent query

The system SHALL format prior turns as a structured prefix and concatenate it with the new query before passing the combined text to the agent runtime. The injection logic applies only to non-ACP code paths; the ACP path retains the runtime's own session memory and SHALL NOT re-prepend history (per `chat-api-acp`).

#### Scenario: History is injected before the current query in the non-ACP path

- **WHEN** a user has N prior turns in the conversation and submits a new query Q via a non-ACP code path
- **THEN** the agent receives a prompt with all prior User/Assistant pairs followed by Q

#### Scenario: ACP path does not double-inject history

- **WHEN** a user submits a query through `POST /api/chat` (ACP path) with `(user_id, conversation_id)` whose pooled subprocess is alive
- **THEN** the server SHALL NOT prepend prior turns; the runtime's own session retains context

#### Scenario: History is capped at 200 most recent turns

- **WHEN** the conversation has more than 200 completed sessions
- **THEN** only the 200 most recent sessions are included in the history prefix (large enough that practical conversations are not truncated; safety cap to bound prompt size)

### Requirement: Client can reset conversation history via new_conversation flag

The POST /api/chat request body SHALL accept an optional boolean field `new_conversation`. When `true`, the server SHALL create a new `conversations` row and use that as the active conversation; the agent SHALL be invoked with no prior context.

#### Scenario: new_conversation creates a fresh conversation row

- **WHEN** the client sends `POST /api/chat` with `new_conversation: true`
- **THEN** the server creates a new `conversations` row and returns its id; the agent receives only the current query

#### Scenario: Omitting new_conversation reuses or resumes the active conversation

- **WHEN** the client sends `POST /api/chat` without `new_conversation` and includes an existing `conversation_id`
- **THEN** the server appends to that conversation and injects (or relies on the runtime session for) prior context as appropriate to the path

### Requirement: Web UI renders all turns in a scrollable thread

The chat frontend SHALL display all turns of the active conversation as a vertically scrolled message thread with alternating user and assistant bubbles. The frontend SHALL paginate via "Load More" against the message-list endpoint when the conversation contains more turns than the initial fetch.

#### Scenario: Multiple turns are visible

- **WHEN** the user has sent two or more messages in the conversation
- **THEN** all prior question/answer pairs are visible above the most recent exchange

#### Scenario: New assistant response is appended

- **WHEN** a new agent response completes
- **THEN** the response is appended to the thread without replacing earlier turns

#### Scenario: Old turns load on demand

- **WHEN** the conversation has more turns than the initial fetch and the user scrolls to the top
- **THEN** the UI fetches older turns via the message-list endpoint and prepends them to the thread

### Requirement: User can start a new conversation from the UI

The frontend SHALL provide a "New conversation" control that creates a fresh conversation, optionally pre-selecting a runtime+model from the user's preference, and clears the local message thread.

#### Scenario: New conversation creates a fresh row and clears the thread

- **WHEN** the user activates the "New conversation" control
- **THEN** the local thread is cleared and the next query creates a new `conversations` row

#### Scenario: New conversation respects user runtime preference

- **WHEN** the user has set a default runtime+model in their preferences
- **THEN** the new conversation row is created with that runtime+model

### Requirement: Discord, Telegram, and scheduler flows are unaffected by web history injection

The system SHALL NOT inject web-chat-style conversation history into agent invocations triggered by Discord messages or Telegram messages. Scheduler-fired prompts in `chat:<convID>` mode are routed through the conversation's ACP session and rely on the runtime's session memory (no separate prefix injection).

#### Scenario: Discord query has no conversation prefix

- **WHEN** an agent session is started from a Discord message
- **THEN** the query is passed to the agent with no web-chat conversation history prefix

#### Scenario: Telegram query has no conversation prefix

- **WHEN** an agent session is started from a Telegram message
- **THEN** the query is passed to the agent with no web-chat conversation history prefix

#### Scenario: Scheduler-fired chat: target uses ACP session memory, not prefix injection

- **WHEN** the scheduler dispatches a `chat:<convID>` job
- **THEN** the prompt is sent to the conversation's ACP session as-is (no prefix); the runtime's session retains context across fires
