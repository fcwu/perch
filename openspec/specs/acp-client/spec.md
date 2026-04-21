## ADDED Requirements

### Requirement: ACP client creates a run and returns a run ID
The system SHALL provide an ACP client that sends a `POST /runs` request to the configured ACP server with the user message as input, and returns a run ID.

#### Scenario: Successful run creation
- **WHEN** `ACP_BASE_URL` is set and the client calls `CreateRun(ctx, message)`
- **THEN** the client sends `POST {ACP_BASE_URL}/runs` with the message payload and returns the run ID from the response

#### Scenario: ACP server unreachable
- **WHEN** the ACP server cannot be reached within the configured timeout
- **THEN** the client returns an error with a descriptive message

### Requirement: ACP client streams run output via SSE
The system SHALL support streaming run output by subscribing to `GET /runs/{id}/stream` (Server-Sent Events) and delivering text chunks to the caller.

#### Scenario: Streaming text output
- **WHEN** a run is in progress and the ACP server emits `MessageOutput` SSE events
- **THEN** the client collects the text content from each event and delivers them to the caller via a channel or callback

#### Scenario: Run completes successfully
- **WHEN** the ACP server emits a `RunCompleted` SSE event (or closes the stream)
- **THEN** the client signals completion and returns the accumulated output text

#### Scenario: Run fails with error
- **WHEN** the ACP server emits a `RunFailed` SSE event
- **THEN** the client returns an error with the failure reason

### Requirement: ACP client respects context cancellation
The system SHALL cancel the ACP run stream when the caller's context is cancelled.

#### Scenario: Context cancelled during streaming
- **WHEN** the caller cancels the context while a run is streaming
- **THEN** the client stops reading the SSE stream and returns a context cancellation error

### Requirement: ACP base URL is configurable via environment variable
The system SHALL read the ACP server URL from the `ACP_BASE_URL` environment variable at startup.

#### Scenario: ACP_BASE_URL is set
- **WHEN** `ACP_BASE_URL=http://localhost:8080` is set in the environment
- **THEN** all ACP client requests are sent to that base URL

#### Scenario: ACP_BASE_URL is not set
- **WHEN** `ACP_BASE_URL` is absent from the environment
- **THEN** the ACP client is not initialized and Discord falls back to PTY mode (if available) or returns a configuration error
