## ADDED Requirements

### Requirement: Optional channel ID configuration
`DISCORD_CHANNEL_ID` SHALL be optional. The system MUST start the Discord Bot when only `DISCORD_BOT_TOKEN` is set.

#### Scenario: Bot starts without channel ID
- **WHEN** `DISCORD_BOT_TOKEN` is set and `DISCORD_CHANNEL_ID` is not set
- **THEN** the Discord Bot starts and listens to all accessible channels

#### Scenario: Bot starts with channel ID (backward compat)
- **WHEN** both `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID` are set
- **THEN** the Discord Bot starts and only listens to the specified channel (existing behavior unchanged)

#### Scenario: Bot does not start without token
- **WHEN** `DISCORD_BOT_TOKEN` is not set
- **THEN** the Discord Bot is not initialized (existing behavior unchanged)

### Requirement: Public Guild channel requires @mention
When no `DISCORD_CHANNEL_ID` filter is active, the Bot in a public Guild text channel (visible to @everyone) SHALL only respond when the Bot is @mentioned in the message.

#### Scenario: Message with mention in public guild channel
- **WHEN** a user sends a message in a public Guild text channel that @mentions the Bot
- **THEN** the Bot processes the message (adds 👀 reaction and writes to PTY)

#### Scenario: Message without mention in public guild channel
- **WHEN** a user sends a message in a public Guild text channel without @mentioning the Bot
- **THEN** the Bot ignores the message silently

#### Scenario: Channel filter overrides mention requirement
- **WHEN** `DISCORD_CHANNEL_ID` is set and a message arrives in that channel without @mention
- **THEN** the Bot processes the message (original behavior, no mention required)

### Requirement: Private Guild channel requires no mention
A private Guild channel (where @everyone's ViewChannel permission is denied) SHALL be treated like a DM — the Bot responds to all messages without requiring @mention.

#### Scenario: Message in private guild channel
- **WHEN** a user sends a message in a private Guild text channel (not visible to @everyone)
- **THEN** the Bot processes the message without requiring @mention

#### Scenario: Private channel type is cached
- **WHEN** the Bot receives a second message from the same private channel
- **THEN** the Bot uses cached channel type without making another API call

### Requirement: DM requires no mention
When a user sends a Direct Message to the Bot, the Bot SHALL respond without requiring @mention.

#### Scenario: Direct message to bot
- **WHEN** a user sends a DM to the Bot
- **THEN** the Bot processes the message and responds

### Requirement: Mention prefix stripped before PTY write
When a Guild channel message triggers the Bot via @mention, the `<@BOT_ID>` prefix SHALL be removed from the message content before writing to PTY.

#### Scenario: Mention prefix removed
- **WHEN** a Guild message `<@1234567890> tell me a joke` triggers the Bot
- **THEN** the PTY receives `tell me a joke` (without the mention prefix)

#### Scenario: Empty content after stripping
- **WHEN** a Guild message contains only the @mention with no additional text
- **THEN** the Bot skips the message (does not write empty content to PTY)

### Requirement: Message Content privileged intent enabled
The Discord Bot SHALL request the `MESSAGE_CONTENT` privileged intent to ensure message content is available for all message types.

#### Scenario: Intent registered at startup
- **WHEN** the Discord Bot starts
- **THEN** `IntentsMessageContent` is included in the session identify intents
