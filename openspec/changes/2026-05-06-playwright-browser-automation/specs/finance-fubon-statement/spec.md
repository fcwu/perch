## ADDED Requirements

### Requirement: Trigger phrase resolves to fubon statement skill

A Claude session inside the container SHALL recognize trigger phrases such as "抓 fubon 帳單", "下載富邦信用卡帳單", "fubon statement" as invocations of the `finance-fubon-statement` skill.

#### Scenario: User asks to download statement

- **WHEN** the user sends "抓 fubon 4 月帳單" in Discord
- **THEN** Claude SHALL invoke the `finance-fubon-statement` skill
- **AND** SHALL NOT begin browser actions before reading the skill's instructions

### Requirement: Skill locates the latest statement email via gws

The skill SHALL use the in-container `gws` CLI to find the most recent Fubon credit-card statement email. It SHALL filter by sender / subject heuristics matching `from:fubon` and Chinese keywords like `信用卡` or `帳單`.

#### Scenario: Latest statement found

- **WHEN** the skill runs and a matching unread or recent email exists in the past 60 days
- **THEN** it SHALL extract the message ID, the statement period (year-month) parsed from the subject, and the HTML body via `gws gmail +read --id <ID> --html`

#### Scenario: No statement email found

- **WHEN** the skill runs and no matching email exists in the search window
- **THEN** it SHALL report "找不到最近的富邦帳單通知信" to the user and stop, without opening a browser

### Requirement: PDF download uses browser-automation skill

The skill SHALL extract the "下載本期帳單(PDF)" anchor URL from the email body, then delegate browser interaction to the `browser-automation` skill. It SHALL inject the national ID and ROC birth date from `/data/secrets/fubon.json`, attempt to solve the CAPTCHA via vision, retry up to 3 times by clicking the regenerate-captcha link, and ask the user via Discord on continued failure.

#### Scenario: Successful download without user intervention

- **WHEN** the CAPTCHA is solved on the first or second attempt and credentials are valid
- **THEN** the bank's "帳單下載" button triggers a PDF download
- **AND** the file SHALL be saved to `/data/finance/fubon/<YYYY>-<MM>.pdf` where `<YYYY>-<MM>` is parsed from the email subject
- **AND** the agent SHALL post a confirmation with the path and file size

#### Scenario: CAPTCHA repeatedly fails

- **WHEN** three consecutive submission attempts fail
- **THEN** the agent SHALL post the current CAPTCHA screenshot to Discord and ask the user to confirm the digits
- **AND** on reply SHALL retry once with the user-supplied value

#### Scenario: Secrets file missing

- **WHEN** `/data/secrets/fubon.json` is absent or fails to parse
- **THEN** the skill SHALL emit an error referencing the expected schema (`{"id": "...", "birth": "0700101"}`)
- **AND** SHALL NOT prompt the user to paste the ID or birthday into Discord

#### Scenario: Statement already archived

- **WHEN** `/data/finance/fubon/<YYYY>-<MM>.pdf` already exists for the period parsed from the email
- **THEN** the skill SHALL report the existing path and size and skip re-downloading
- **AND** SHALL NOT delete or overwrite the existing file

### Requirement: PDF is archived encrypted

The skill SHALL save the PDF as-is (password-protected by Fubon with the user's national ID). It SHALL NOT attempt to decrypt or re-encrypt the file.

#### Scenario: Encrypted PDF preserved

- **WHEN** the PDF is downloaded and saved
- **THEN** opening the saved file SHALL still require the national ID password
- **AND** the skill SHALL NOT log or store the password
