## Requirements

### Requirement: Detect workspace as git repository
System SHALL check whether `/workspace` (or `WORKSPACE_PATH` env var) is a valid git repository at startup.

#### Scenario: Valid git repo detected
- **WHEN** `/workspace/.git` directory exists
- **THEN** system starts the auto-sync background loop

#### Scenario: Not a git repo
- **WHEN** `/workspace/.git` does not exist
- **THEN** system skips sync setup and logs an info message

### Requirement: Periodic pull and push sync
System SHALL run `git pull --rebase` followed by `git push` on the configured interval (default 60 seconds).

#### Scenario: Successful sync
- **WHEN** the sync interval elapses and workspace is clean
- **THEN** system executes `git pull --rebase` then `git push` and logs success

#### Scenario: Sync with dirty workspace
- **WHEN** the sync interval elapses and workspace has uncommitted changes
- **THEN** system runs `git stash`, then `git pull --rebase`, then `git stash pop`, then `git push`

#### Scenario: Stash pop conflict
- **WHEN** `git stash pop` fails due to conflict
- **THEN** system logs the stash ref and error, skips `git push`, does NOT abort the stash

### Requirement: Rebase conflict handling
System SHALL detect an ongoing rebase state and abort it before the next sync attempt.

#### Scenario: Rebase conflict detected
- **WHEN** `.git/rebase-merge` or `.git/rebase-apply` exists at sync time
- **THEN** system runs `git rebase --abort`, logs the event, and skips push for that cycle

### Requirement: Configurable sync via environment variables
System SHALL read sync behaviour from environment variables.

#### Scenario: Sync disabled by default
- **WHEN** `WORKSPACE_GIT_SYNC_ENABLED` is not set or is `false`
- **THEN** sync loop does not start

#### Scenario: Sync enabled explicitly
- **WHEN** `WORKSPACE_GIT_SYNC_ENABLED=true`
- **THEN** sync loop starts after workspace detection

#### Scenario: Custom interval
- **WHEN** `WORKSPACE_GIT_SYNC_INTERVAL=120` is set
- **THEN** sync loop runs every 120 seconds

### Requirement: Push failure handling
System SHALL handle `git push` failures without crashing.

#### Scenario: Push rejected
- **WHEN** `git push` exits with non-zero status
- **THEN** system logs the error output and continues the sync loop on next tick

### Requirement: Discord notification on sync failure
System SHALL send a Discord notification when a sync error occurs, with debounce to avoid repeated messages.

#### Scenario: Rebase conflict notification
- **WHEN** rebase conflict is detected and aborted
- **THEN** system sends a Discord message indicating the conflict and instructs the user to resolve manually

#### Scenario: Stash pop conflict notification
- **WHEN** `git stash pop` fails
- **THEN** system sends a Discord message with the stash ref and instructions

#### Scenario: Push failure notification
- **WHEN** `git push` fails
- **THEN** system sends a Discord message with the error output

#### Scenario: Debounce repeated failures
- **WHEN** the same error type occurs within 5 minutes of a previous notification
- **THEN** system does NOT send another Discord notification

#### Scenario: Notification sent to configured notify channel
- **WHEN** `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL=<channel_id>` is set and a sync error occurs
- **THEN** system sends the notification to that channel only

#### Scenario: No notify channel configured
- **WHEN** `WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL` is not set
- **THEN** system only logs the error locally, no Discord notification is sent

### Requirement: Structured logging for all git operations
System SHALL emit structured slog entries before and after every git command execution.

#### Scenario: Rebase conflict full log
- **WHEN** rebase conflict is detected and aborted
- **THEN** slog MUST contain: detection log (Warn), `git rebase --abort` stdout+stderr (Info), and abort result log (Info on success, Error on failure)

#### Scenario: Stash pop failure full log
- **WHEN** `git stash pop` fails
- **THEN** slog MUST contain: stash ref, full error output, and indication that push was skipped

#### Scenario: Push failure full log
- **WHEN** `git push` fails
- **THEN** slog MUST contain: full stdout+stderr from git push and the exit error

#### Scenario: Credential injection log
- **WHEN** `WORKSPACE_GIT_TOKEN` is set and credential injection runs
- **THEN** slog MUST log each step (host detection, helper config, write result) WITHOUT logging the token value itself

### Requirement: Git token credential injection
System SHALL configure git credential from `WORKSPACE_GIT_TOKEN` environment variable at startup.

#### Scenario: Token provided
- **WHEN** `WORKSPACE_GIT_TOKEN=<token>` is set and remote is HTTPS
- **THEN** system writes token to `~/.git-credentials` and sets `credential.helper=store` before starting sync

#### Scenario: Token not provided
- **WHEN** `WORKSPACE_GIT_TOKEN` is not set
- **THEN** system skips credential injection and relies on system credential helper

#### Scenario: SSH remote with token set
- **WHEN** remote URL is `git@...` (SSH) and `WORKSPACE_GIT_TOKEN` is set
- **THEN** system ignores the token (SSH key takes precedence) and logs a warning
