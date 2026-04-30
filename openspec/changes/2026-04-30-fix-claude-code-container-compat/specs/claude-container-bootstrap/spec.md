## ADDED Requirements

### Requirement: Entrypoint copies host Claude config to a writable container-local copy

When the perch container is started with the host `~/.claude` directory bind-mounted read-only at the staging path `/etc/perch-claude-host`, the entrypoint SHALL copy its contents to `/home/perchuser/.claude/` so that Claude Code 2.1.x can rename `plugins/*.bak`, create `session-env/<uuid>/`, and otherwise mutate the directory without EROFS, without modifying the host config.

#### Scenario: host claude staging is mounted

- **WHEN** the container starts with `${HOME}/.claude` bind-mounted to `/etc/perch-claude-host:ro`
- **THEN** the entrypoint creates `/home/perchuser/.claude/` and runs `cp -a /etc/perch-claude-host/. /home/perchuser/.claude/`
- **AND** the local copy is owned by `${PUID}:${PGID}` after copy
- **AND** Claude Code's subsequent `plugins/*.bak` rename and `session-env/<uuid>/` mkdir succeed in the local copy
- **AND** the host `~/.claude` is never written to

#### Scenario: volatile and personal sub-paths are excluded from the copy

- **WHEN** `cp -a` completes
- **THEN** the entrypoint deletes from `/home/perchuser/.claude/` the following entries (if present): `sessions/`, `projects/`, `cache/`, `debug/`, `backups/`, `shell-snapshots/`, `history.jsonl`
- **AND** the deletions do not affect host `~/.claude` (only the container-local copy)
- **AND** preserved entries include `settings.json`, `settings.local.json`, `plugins/`, `skills/`, `.credentials.json`, `statusline-command.sh`

#### Scenario: host claude staging is NOT mounted (fresh container)

- **WHEN** `/etc/perch-claude-host` does not exist
- **THEN** the entrypoint creates an empty `/home/perchuser/.claude/` directory owned by `${PUID}:${PGID}`
- **AND** does not abort startup
- **AND** logs an info-level message indicating fresh-init mode
- **AND** the `.claude.json` seed step (separate Requirement) still runs

#### Scenario: cp fails (disk full or staging is malformed)

- **WHEN** `cp -a` exits non-zero
- **THEN** the entrypoint logs an error naming the failing path and exit code
- **AND** does not abort container startup
- **AND** continues to subsequent steps (seed, merge, exec perch)

### Requirement: Entrypoint seeds Claude Code onboarding flags into a fresh `.claude.json`

When the container starts, the entrypoint SHALL ensure that `~/.claude.json` contains the minimum onboarding flags required by Claude Code 2.1.x to bypass the interactive theme dialog and the workspace trust prompt, so that interactive PTY sessions (`claude --permission-mode bypassPermissions --name <id>`) do not block on first run.

#### Scenario: `.claude.json` is missing entirely

- **WHEN** `/home/perchuser/.claude.json` does not exist at entrypoint start
- **THEN** the entrypoint creates it with `{}` and proceeds to seed onboarding fields

#### Scenario: onboarding fields are missing or null

- **WHEN** any of `hasCompletedOnboarding`, `theme`, `hasAcceptedAllTerms`, or `projects["<workspace>"].hasTrustDialogAccepted` is missing or null
- **THEN** the entrypoint sets the missing field(s) to safe default values:
  - `hasCompletedOnboarding = true`
  - `theme = "dark-daltonized"` (or the documented Claude Code 2.1.x default)
  - `hasAcceptedAllTerms = true`
  - `projects["<workspace>"].hasTrustDialogAccepted = true`
- **AND** other fields in `.claude.json` are preserved unchanged
- **AND** the file ends up owned by `${PUID}:${PGID}` when `PUID` is set

#### Scenario: existing onboarding values (including `false`) are preserved

- **WHEN** `.claude.json` already has `hasAcceptedAllTerms = false`
- **THEN** the entrypoint does NOT overwrite the field
- **AND** logs that user-set onboarding values are being kept

#### Scenario: jq is unavailable

- **WHEN** the image is missing the `jq` binary
- **THEN** the entrypoint logs an error explaining that `.claude.json` seeding is skipped
- **AND** does not abort startup

### Requirement: Entrypoint failure modes do not block container startup

When any bootstrap helper step (tmpfs check, `.claude.json` seed, hook merge) fails, the entrypoint SHALL log a clear warning identifying the failed step and continue to `exec /app/perch`, so that misconfiguration does not prevent perch from starting.

#### Scenario: jq seed fails on malformed `.claude.json`

- **WHEN** `~/.claude.json` exists but is not valid JSON
- **THEN** the entrypoint logs an error naming the file and the underlying error
- **AND** does not modify the file
- **AND** continues to `exec /app/perch`

#### Scenario: merge-settings node script crashes

- **WHEN** `node /app/perch-claude/merge-settings.js` exits non-zero for either target
- **THEN** the entrypoint logs the exit code and target path
- **AND** continues to `exec /app/perch`

#### Scenario: tmpfs check finds neither RO nor RW (mount inspection failed)

- **WHEN** `mountpoint` / write-test detection cannot determine the mount mode of `~/.claude`
- **THEN** the entrypoint logs an info-level message stating the mount mode is unknown
- **AND** skips the tmpfs warning
- **AND** continues to `exec /app/perch`
