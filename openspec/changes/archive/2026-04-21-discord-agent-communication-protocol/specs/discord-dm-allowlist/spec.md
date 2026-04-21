## MODIFIED Requirements

### Requirement: DM allowlist via environment variable
The system SHALL read `DISCORD_ALLOWED_USER_IDS` at startup as a comma-separated list of Discord user IDs. Only users whose ID appears in this list MAY interact with the Bot via DM.

#### Scenario: Allowlist configured with one user
- **WHEN** `DISCORD_ALLOWED_USER_IDS=123456789` is set
- **THEN** only the user with ID `123456789` can interact via DM; all other DMs are silently ignored

#### Scenario: Allowlist configured with multiple users
- **WHEN** `DISCORD_ALLOWED_USER_IDS=111,222,333` is set
- **THEN** users with IDs `111`, `222`, and `333` can all interact via DM

#### Scenario: Allowlist not set
- **WHEN** `DISCORD_ALLOWED_USER_IDS` is not set or is empty
- **THEN** all DMs are silently ignored (deny-by-default)

#### Scenario: Unknown user sends DM
- **WHEN** a user whose ID is not in the allowlist sends a DM
- **THEN** the Bot does not add any reaction and does not trigger an ACP run or PTY write
