## ADDED Requirements

### Requirement: Discord messages forwarded to PTY
When `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID` are set, Perch SHALL start a Discord bot that listens to the specified channel and writes received messages to the PTY.

#### Scenario: Discord message received
- **WHEN** a user sends a message in the configured Discord channel
- **THEN** the message text is written to the PTY followed by a newline
- **THEN** a 👀 reaction is added to the original Discord message

#### Scenario: Discord message from other channel ignored
- **WHEN** a message arrives in a channel other than `DISCORD_CHANNEL_ID`
- **THEN** the message is silently ignored

#### Scenario: Discord bot not started without token
- **WHEN** `DISCORD_BOT_TOKEN` is not set
- **THEN** Perch starts normally without Discord bot

### Requirement: Telegram messages forwarded to PTY
When `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are set, Perch SHALL start a Telegram bot that listens to the specified chat and writes received messages to the PTY.

#### Scenario: Telegram message received
- **WHEN** a user sends a message in the configured Telegram chat
- **THEN** the message text is written to the PTY followed by a newline

#### Scenario: Telegram message from other chat ignored
- **WHEN** a message arrives from a chat other than `TELEGRAM_CHAT_ID`
- **THEN** the message is silently ignored

#### Scenario: Telegram bot not started without token
- **WHEN** `TELEGRAM_BOT_TOKEN` is not set
- **THEN** Perch starts normally without Telegram bot
