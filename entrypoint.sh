#!/bin/sh
set -e

if [ -n "$PUID" ]; then
    PGID="${PGID:-$PUID}"
    mkdir -p /home/perchuser
    chown "${PUID}:${PGID}" /home/perchuser
    export HOME=/home/perchuser
fi

# Determine workspace directory (same logic as main.go)
WORKDIR="${CLAUDE_WORKDIR}"
if [ -z "$WORKDIR" ] && [ -d /workspace ]; then
    WORKDIR="/workspace"
fi

if [ -n "$WORKDIR" ] && [ -n "$PUID" ]; then
    # Pre-create .perch with PUID:PGID ownership so the scheduler's MkdirAll
    # is a no-op and the directory is writable by the non-root claude process.
    mkdir -p "$WORKDIR/.perch"
    chown "${PUID}:${PGID}" "$WORKDIR/.perch"
fi

# Copy host ~/.claude to a writable local copy so Claude Code 2.1.x can
# mutate plugins/*.bak and session-env/<uuid>/ without hitting EROFS on the
# RO bind mount.  Mount convention: ${HOME}/.claude:/etc/perch-claude-host:ro
#
# Preserve any credentials written by a previous in-container `claude /login`
# (stored in the persistent /home/perchuser volume).  The cp below would
# overwrite them with the host's potentially stale copy, so we rescue them
# first and restore them afterwards.
_local_creds="/home/perchuser/.claude/.credentials.json"
_saved_creds=""
if [ -f "$_local_creds" ]; then
    _saved_creds=$(cat "$_local_creds")
fi

if [ -d /etc/perch-claude-host ]; then
    if mkdir -p /home/perchuser/.claude && \
       cp -a /etc/perch-claude-host/. /home/perchuser/.claude/ 2>/dev/null; then
        # Strip volatile / large / sensitive dirs that must not carry over from host.
        rm -rf \
            /home/perchuser/.claude/sessions \
            /home/perchuser/.claude/projects \
            /home/perchuser/.claude/cache \
            /home/perchuser/.claude/debug \
            /home/perchuser/.claude/backups \
            /home/perchuser/.claude/shell-snapshots \
            /home/perchuser/.claude/history.jsonl
    else
        echo "perch entrypoint: warning: cp /etc/perch-claude-host failed, continuing with empty ~/.claude" >&2
        mkdir -p /home/perchuser/.claude
    fi
else
    # No staging mount — start with an empty local dir; user will claude /login.
    mkdir -p /home/perchuser/.claude
fi

# Restore in-container credentials if they existed before the cp above.
# These take priority over the host's copy so that `claude /login` run inside
# the container persists across restarts (the /home/perchuser volume is persistent).
if [ -n "$_saved_creds" ]; then
    echo "$_saved_creds" > "$_local_creds"
    echo "perch entrypoint: restored in-container credentials ($(wc -c < "$_local_creds") bytes)" >&2
fi

# Seed ~/.claude.json with required onboarding flags for fresh containers.
# Only fills missing/null fields; existing values (including false) are kept.
CLAUDE_JSON="/home/perchuser/.claude.json"
if [ ! -f "$CLAUDE_JSON" ]; then
    echo '{}' > "$CLAUDE_JSON"
fi
if command -v jq >/dev/null 2>&1; then
    _wd="${WORKDIR:-/workspace}"
    _bpath="${PLAYWRIGHT_BROWSERS_PATH:-/opt/ms-playwright}"
    # @playwright/mcp creates a state dir inside PLAYWRIGHT_BROWSERS_PATH at runtime.
    # /opt/ms-playwright is read-only for non-root PUID users, so point the writable
    # state path at /data/playwright and use --executable-path to reference the binary
    # directly from /opt/ms-playwright (read access is sufficient for launching).
    _chrome_exec=$(find "$_bpath" -name "chrome" -path "*/chrome-linux64/chrome" 2>/dev/null | head -1)
    _mcp_state="/data/playwright"
    _mcp_output="/tmp/playwright-output"
    _mcp_args="[\"-y\", \"@playwright/mcp\", \"--headless\", \"--browser=chromium\", \"--no-sandbox\", \"--output-dir=$_mcp_output\"]"
    if [ -n "$_chrome_exec" ]; then
        _mcp_args="[\"-y\", \"@playwright/mcp\", \"--headless\", \"--browser=chromium\", \"--no-sandbox\", \"--output-dir=$_mcp_output\", \"--executable-path=$_chrome_exec\"]"
    fi
    jq --arg wd "$_wd" --arg mcp_state "$_mcp_state" --argjson mcp_args "$_mcp_args" '
        if .hasCompletedOnboarding == null then .hasCompletedOnboarding = true else . end |
        if .theme == null then .theme = "dark-daltonized" else . end |
        if .hasAcceptedAllTerms == null then .hasAcceptedAllTerms = true else . end |
        if .projects[$wd].hasTrustDialogAccepted == null then
            .projects[$wd].hasTrustDialogAccepted = true
        else . end |
        if (.mcpServers.playwright == null) or
           (.mcpServers.playwright.env.PLAYWRIGHT_BROWSERS_PATH != $mcp_state) or
           ((.mcpServers.playwright.args // []) | contains(["--no-sandbox"]) | not) or
           ((.mcpServers.playwright.args // []) | contains(["--output-dir=/tmp/playwright-output"]) | not) then
            .mcpServers.playwright = {
                "command": "npx",
                "args": $mcp_args,
                "env": {"PLAYWRIGHT_BROWSERS_PATH": $mcp_state}
            }
        else . end
    ' "$CLAUDE_JSON" > "${CLAUDE_JSON}.tmp" && mv "${CLAUDE_JSON}.tmp" "$CLAUDE_JSON"

    # Patch the official Playwright plugin .mcp.json so it uses the same config,
    # overriding any user/agent edits that may have broken it.
    #
    # NOTE: The Claude Code plugin marketplace re-syncs this file shortly after
    # perch starts (when the first claude-agent-acp subprocess initialises), so
    # a one-shot patch here loses the race.  Stage a long-form patch in a tmp
    # script and have the post-startup loop (just before exec perch) keep
    # rewriting the file until it stops getting reset.
    _playwright_plugin="/home/perchuser/.claude/plugins/marketplaces/claude-plugins-official/external_plugins/playwright/.mcp.json"
    if [ -f "$_playwright_plugin" ]; then
        jq --arg mcp_state "$_mcp_state" --argjson mcp_args "$_mcp_args" '
            .playwright = {
                "command": "npx",
                "args": $mcp_args,
                "env": {"PLAYWRIGHT_BROWSERS_PATH": $mcp_state}
            }
        ' "$_playwright_plugin" > "${_playwright_plugin}.tmp" && mv "${_playwright_plugin}.tmp" "$_playwright_plugin"

        # Cache the patched payload so the background re-applier can restore
        # it whenever claude resets the file.
        cp "$_playwright_plugin" /tmp/perch-playwright-mcp.json
    fi
else
    echo "perch entrypoint: warning: jq not found, skipping .claude.json seed" >&2
fi

if [ -n "$WORKDIR" ]; then
    AGENT_RUNTIME="${AGENT_RUNTIME:-claude}"
    if [ "$AGENT_RUNTIME" = "opencode" ]; then
        if [ -d /app/perch-opencode/skills ]; then
            mkdir -p "$WORKDIR/.opencode/skills"
            for skill_dir in /app/perch-opencode/skills/*/; do
                skill_name=$(basename "$skill_dir")
                if [ ! -d "$WORKDIR/.opencode/skills/$skill_name" ]; then
                    cp -r "$skill_dir" "$WORKDIR/.opencode/skills/$skill_name"
                fi
            done
        fi
        if [ -n "$PUID" ] && [ -d "$WORKDIR/.opencode" ]; then
            chown -R "${PUID}:${PGID}" "$WORKDIR/.opencode"
        fi
    elif [ "$AGENT_RUNTIME" = "codex" ]; then
        # codex-acp reads ~/.codex/config.toml; perch ships /app/perch-codex/
        # as a placeholder asset dir (no skills today; future change may add).
        if [ -d /app/perch-codex/skills ] && [ -n "$(ls -A /app/perch-codex/skills 2>/dev/null)" ]; then
            mkdir -p "$WORKDIR/.codex/skills"
            for skill_dir in /app/perch-codex/skills/*/; do
                [ -d "$skill_dir" ] || continue
                skill_name=$(basename "$skill_dir")
                if [ ! -d "$WORKDIR/.codex/skills/$skill_name" ]; then
                    cp -r "$skill_dir" "$WORKDIR/.codex/skills/$skill_name"
                fi
            done
        fi
        if [ -n "$PUID" ] && [ -d "$WORKDIR/.codex" ]; then
            chown -R "${PUID}:${PGID}" "$WORKDIR/.codex"
        fi
    else
        # Copy perch-bundled skills into $WORKDIR/.claude/skills/ (always overwrite).
        # Claude Code discovers skills from both ~/.claude/skills/ (global) and
        # .claude/skills/ inside the working directory (project-level), so this
        # avoids touching the host-mounted ~/.claude entirely.
        # Always overwrite so updated SKILL.md files take effect after image upgrades.
        if [ -d /app/perch-claude/skills ]; then
            mkdir -p "$WORKDIR/.claude/skills"
            for skill_dir in /app/perch-claude/skills/*/; do
                skill_name=$(basename "$skill_dir")
                cp -rf "$skill_dir" "$WORKDIR/.claude/skills/$skill_name"
            done
        fi


        # Seed workspace CLAUDE.md with perch-specific instructions (no-overwrite).
        if [ ! -f "$WORKDIR/CLAUDE.md" ] && [ -f /app/perch-claude/CLAUDE.md ]; then
            cp /app/perch-claude/CLAUDE.md "$WORKDIR/CLAUDE.md"
        fi

        # Fix ownership of anything created above under .claude/ (mkdir/cp/node all
        # run as root).  Only applies when PUID is set; harmless if .claude/ doesn't
        # exist yet.
        if [ -n "$PUID" ] && [ -d "$WORKDIR/.claude" ]; then
            chown -R "${PUID}:${PGID}" "$WORKDIR/.claude"
        fi
        if [ -n "$PUID" ] && [ -f "$WORKDIR/CLAUDE.md" ]; then
            chown "${PUID}:${PGID}" "$WORKDIR/CLAUDE.md"
        fi
    fi
fi

# Fix ownership of user-level claude config (cp and seed above ran as root).
if [ -n "$PUID" ]; then
    [ -d /home/perchuser/.claude ] && chown -R "${PUID}:${PGID}" /home/perchuser/.claude
    [ -f "$CLAUDE_JSON" ] && chown "${PUID}:${PGID}" "$CLAUDE_JSON"
fi

# Ensure browser automation and finance data directories exist under /data.
# These are created at startup so container-side skills can write immediately.
mkdir -p /data/playwright/profile /data/playwright/downloads /data/playwright/state
mkdir -p /data/finance
# /data itself must be writable by PUID so perch can create perch.db there.
# When /data is a freshly created docker volume it's owned by root:root.
if [ -n "$PUID" ]; then
    chown "${PUID}:${PGID}" /data
    chown -R "${PUID}:${PGID}" /data/playwright /data/finance
fi
# Screenshot output dir must be writable by PUID so @playwright/mcp can save files there.
mkdir -p /tmp/playwright-output
if [ -n "$PUID" ]; then
    chown "${PUID}:${PGID}" /tmp/playwright-output
fi
# Remove stale Chrome SingletonLock files left over from crashed/killed sessions.
# On fresh container start there are no live Chrome processes, so any lock is stale.
find /data/playwright -name "SingletonLock" -delete 2>/dev/null || true
if [ ! -d /data/secrets ]; then
    mkdir -p /data/secrets
    chmod 0700 /data/secrets
fi

AUTH_METHOD="${AUTH_METHOD:-${AUTH_MODE:-none}}"
export AUTH_METHOD

# Background re-applier for the Playwright plugin .mcp.json.
# Claude Code's plugin marketplace overwrites this file ~2-5s after the first
# claude-agent-acp subprocess starts, reverting our patch.  Loop in the
# background, restoring the file whenever its size shrinks below the patched
# payload (87B "npx @playwright/mcp@latest" template = unpatched).
_pw_plg="/home/perchuser/.claude/plugins/marketplaces/claude-plugins-official/external_plugins/playwright/.mcp.json"
if [ -f /tmp/perch-playwright-mcp.json ] && [ -f "$_pw_plg" ]; then
    (
        # Run forever — claude marketplace may resync the file at any time
        # (on each ACP session start, plugin update, etc.).  Keep watching as
        # long as perch is alive.
        while :; do
            sleep 5
            cur=$(wc -c < "$_pw_plg" 2>/dev/null || echo 0)
            want=$(wc -c < /tmp/perch-playwright-mcp.json 2>/dev/null || echo 0)
            if [ "$cur" != "$want" ]; then
                cp /tmp/perch-playwright-mcp.json "$_pw_plg" 2>/dev/null || true
                if [ -n "$PUID" ]; then
                    chown "${PUID}:${PGID}" "$_pw_plg" 2>/dev/null || true
                fi
            fi
        done
    ) &
fi

if [ -n "$PUID" ]; then
    exec gosu "${PUID}:${PGID}" env HOME=/home/perchuser /app/perch
fi
exec /app/perch
