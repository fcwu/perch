## MODIFIED Requirements

### Requirement: DM requires no mention
When a user sends a Direct Message to the Bot, the Bot SHALL respond without requiring @mention, **provided the sender's user ID is in the `DISCORD_ALLOWED_USER_IDS` allowlist**. If no allowlist is configured, all DMs SHALL be silently ignored.

#### Scenario: Direct message from allowlisted user
- **WHEN** a user in the allowlist sends a DM to the Bot
- **THEN** the Bot processes the message and responds via ACP run (if `ACP_BASE_URL` is set) or PTY session (fallback)

#### Scenario: Direct message from non-allowlisted user
- **WHEN** a user NOT in the allowlist sends a DM to the Bot
- **THEN** the Bot silently ignores the message (no reaction, no ACP run, no PTY write)

#### Scenario: DM with no allowlist configured
- **WHEN** `DISCORD_ALLOWED_USER_IDS` is not set and a user sends a DM
- **THEN** the Bot silently ignores the message
