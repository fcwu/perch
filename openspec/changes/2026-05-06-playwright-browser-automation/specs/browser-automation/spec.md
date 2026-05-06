## ADDED Requirements

### Requirement: Playwright MCP server runs headless inside container

The perch runtime container SHALL ship a Chromium binary, Playwright libraries, and the `@playwright/mcp` server. The MCP server SHALL be exposed to the Claude runtime via Claude's MCP configuration with `--headless`, `--browser=chromium`, and a `--user-data-dir` rooted under `/data/playwright/profile/`.

#### Scenario: Image contains required binaries

- **WHEN** the runtime image is built and inspected
- **THEN** `npx @playwright/mcp --version` SHALL succeed
- **AND** `npx playwright install --dry-run chromium` SHALL report Chromium already installed
- **AND** CJK fonts (`fonts-noto-cjk`) SHALL be installed

#### Scenario: Claude advertises browser_* tools

- **WHEN** a Claude session boots inside the container
- **THEN** the available MCP tools SHALL include `browser_navigate`, `browser_take_screenshot`, `browser_click`, `browser_type`, and at least one download-handling tool

#### Scenario: Profile is per-conversation

- **WHEN** two concurrent Claude sessions with different `PERCH_CONV_ID` values both invoke browser tools
- **THEN** each session SHALL bind to a distinct `--user-data-dir` under `/data/playwright/profile/<conv-id>/`
- **AND** neither session's cookies / localStorage SHALL be visible to the other

### Requirement: Volume layout for browser persistence

The container SHALL define and create the following directories under `/data` if they do not exist:

- `/data/playwright/profile/` — per-conversation Chromium profiles
- `/data/playwright/downloads/` — default download landing area
- `/data/playwright/state/` — shared `storageState.json` files seeded from outside the container
- `/data/secrets/` — sensitive credential files, mode 0600
- `/data/finance/` — archived financial documents

#### Scenario: Entrypoint creates directories on cold start

- **WHEN** the container starts and `/data/playwright/` does not exist
- **THEN** entrypoint SHALL create `/data/playwright/{profile,downloads,state}` and `/data/secrets/` and `/data/finance/`
- **AND** `/data/secrets/` SHALL be `chmod 0700`
- **AND** any pre-existing files SHALL NOT be modified

### Requirement: Human-in-the-loop via Discord screenshot

When the browser-automation skill cannot proceed autonomously (e.g. CAPTCHA fails 3+ retries, unexpected page, missing element), the agent SHALL post the current screenshot and a specific question into the active Discord conversation and wait for the user's reply before continuing. The agent SHALL NOT silently fail or loop.

#### Scenario: CAPTCHA solved by Claude on first try

- **WHEN** a login page presents a digit CAPTCHA the agent's vision can resolve confidently
- **THEN** the agent fills the digits and submits without involving the user
- **AND** no screenshot is posted to Discord for confirmation

#### Scenario: CAPTCHA fails repeatedly

- **WHEN** three consecutive submissions fail with a CAPTCHA-style error
- **THEN** the agent SHALL take a fresh screenshot of the CAPTCHA element and post it with a message asking the user to confirm the digits
- **AND** SHALL pause until the user replies
- **AND** on reply, SHALL submit the user-supplied value

#### Scenario: Unexpected page

- **WHEN** the browser navigates to a page whose structure does not match the skill's expected selectors
- **THEN** the agent SHALL post a full-page screenshot plus a textual description of what it expected vs. what it sees
- **AND** SHALL NOT attempt random clicks

### Requirement: Sensitive credentials are not logged in plain text

Skills SHALL inject credentials from `/data/secrets/<site>.json` into form fields without writing the plaintext value into the conversation transcript or any tool argument that is persisted in clear text. The skill instructions SHALL phrase the action as "filled `<field>` from secrets" rather than echoing the value.

#### Scenario: ID injection from secrets

- **WHEN** a skill needs to fill a national ID field
- **THEN** the skill instruction SHALL direct Claude to use a shell expansion like `$(jq -r .id /data/secrets/<site>.json)` as the `text` argument
- **AND** Claude's user-visible narration SHALL NOT contain the literal ID value

#### Scenario: Missing secrets file

- **WHEN** a skill expects `/data/secrets/<site>.json` and the file does not exist
- **THEN** the skill SHALL emit a structured error explaining what file is needed and what schema it should contain
- **AND** SHALL NOT prompt the user to paste the secret into the chat

### Requirement: storageState bootstrap is host-driven

For sites whose first-time login cannot be completed inside the container (multi-step OAuth, biometrics, captchas the agent cannot solve), the operator SHALL run a host-side script (`tests/playwright-login.sh <site>`) that drives a headed Playwright session on the operator's workstation, captures `storageState.json`, and copies it into the container at `/data/playwright/state/<site>.json`. The container-side skills SHALL NOT attempt headed execution and SHALL NOT prompt for plaintext credentials inside Discord.

#### Scenario: Site requires storageState but file is missing

- **WHEN** a skill checks for `/data/playwright/state/<site>.json` and it is absent
- **THEN** the skill SHALL emit a message instructing the user to run `tests/playwright-login.sh <site>` from their workstation
- **AND** SHALL NOT attempt to log the user in via Discord prompts

#### Scenario: storageState present and valid

- **WHEN** `/data/playwright/state/<site>.json` exists and the skill launches a browser context with `storage_state=<path>`
- **THEN** the resulting page SHALL present the post-login state without re-authentication

### Requirement: Headless fallback ladder is documented but not pre-implemented

The DEVELOPMENT.md SHALL document a fallback ladder for sites that block headless automation: (0) plain headless, (1) headless + persistent user-data-dir, (2) headless + stealth args, (3) Xvfb + headed inside container, (4) host-side execution with state sync. Levels 2–4 SHALL NOT be implemented in this change; they SHALL be added only when an actual site triggers the need.
