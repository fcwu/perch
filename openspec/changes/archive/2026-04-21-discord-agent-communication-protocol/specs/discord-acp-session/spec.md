## ADDED Requirements

### Requirement: Discord message triggers an ACP run
When the Discord session is in ACP mode, an inbound Discord message SHALL trigger a new ACP run instead of writing to a PTY session.

#### Scenario: Message creates ACP run
- **WHEN** a validated Discord message arrives and `ACP_BASE_URL` is configured
- **THEN** the session calls `ACPClient.CreateRun()` with the message content and channel ID as metadata

#### Scenario: ACP run not started when ACP_BASE_URL is absent
- **WHEN** `ACP_BASE_URL` is not set
- **THEN** the Discord session uses the existing PTY mode and does not call the ACP client

### Requirement: Discord session reflects ACP run status with emoji reactions
While the ACP run is in progress, the session SHALL update Discord emoji reactions to indicate working state, and update them again upon completion.

#### Scenario: Run starts — eyes reaction added
- **WHEN** the ACP run is created successfully
- **THEN** the 👀 reaction is added to the user's message

#### Scenario: Run completes — final reaction updated
- **WHEN** the ACP run finishes with success
- **THEN** the 👀 reaction is removed and 💬 is added to the message

#### Scenario: Run fails — error reaction shown
- **WHEN** the ACP run finishes with an error
- **THEN** the 👀 reaction is removed and ❌ is added to the message

### Requirement: ACP run output is sent to Discord as a formatted message
Upon ACP run completion, the accumulated text output SHALL be formatted and sent to the Discord channel using the existing output formatting rules.

#### Scenario: Output sent after run completion
- **WHEN** the ACP run SSE stream closes with success
- **THEN** the accumulated output text is split into ≤1900-character chunks and each chunk is sent as a Discord message to the originating channel

#### Scenario: Empty output handled gracefully
- **WHEN** the ACP run completes but returns no text output
- **THEN** no Discord message is sent (no empty message posted)

#### Scenario: Tables in output wrapped in code blocks
- **WHEN** the ACP run output contains table-formatted text
- **THEN** the output is wrapped in a code block with CJK-aware column alignment before sending to Discord

### Requirement: ACP run timeout is enforced per message
Each ACP run triggered by a Discord message SHALL be subject to a configurable timeout.

#### Scenario: Run completes within timeout
- **WHEN** the ACP run finishes before the timeout expires
- **THEN** the output is sent normally to Discord

#### Scenario: Run exceeds timeout
- **WHEN** the ACP run does not complete within the configured timeout
- **THEN** the session cancels the run, removes the 👀 reaction, adds ❌, and sends a timeout error message to Discord
