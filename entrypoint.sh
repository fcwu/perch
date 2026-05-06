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

# Seed ~/.claude.json with required onboarding flags for fresh containers.
# Only fills missing/null fields; existing values (including false) are kept.
CLAUDE_JSON="/home/perchuser/.claude.json"
if [ ! -f "$CLAUDE_JSON" ]; then
    echo '{}' > "$CLAUDE_JSON"
fi
if command -v jq >/dev/null 2>&1; then
    _wd="${WORKDIR:-/workspace}"
    jq --arg wd "$_wd" '
        if .hasCompletedOnboarding == null then .hasCompletedOnboarding = true else . end |
        if .theme == null then .theme = "dark-daltonized" else . end |
        if .hasAcceptedAllTerms == null then .hasAcceptedAllTerms = true else . end |
        if .projects[$wd].hasTrustDialogAccepted == null then
            .projects[$wd].hasTrustDialogAccepted = true
        else . end |
        if .mcpServers.playwright == null then
            .mcpServers.playwright = {
                "command": "npx",
                "args": ["-y", "@playwright/mcp", "--headless", "--browser=chromium",
                         "--user-data-dir=/data/playwright/profile"]
            }
        else . end
    ' "$CLAUDE_JSON" > "${CLAUDE_JSON}.tmp" && mv "${CLAUDE_JSON}.tmp" "$CLAUDE_JSON"
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
        # Copy perch-bundled skills into $WORKDIR/.claude/skills/ (no-overwrite).
        # Claude Code discovers skills from both ~/.claude/skills/ (global) and
        # .claude/skills/ inside the working directory (project-level), so this
        # avoids touching the host-mounted ~/.claude entirely.
        if [ -d /app/perch-claude/skills ]; then
            mkdir -p "$WORKDIR/.claude/skills"
            for skill_dir in /app/perch-claude/skills/*/; do
                skill_name=$(basename "$skill_dir")
                if [ ! -d "$WORKDIR/.claude/skills/$skill_name" ]; then
                    cp -r "$skill_dir" "$WORKDIR/.claude/skills/$skill_name"
                fi
            done
        fi


        # Fix ownership of anything created above under .claude/ (mkdir/cp/node all
        # run as root).  Only applies when PUID is set; harmless if .claude/ doesn't
        # exist yet.
        if [ -n "$PUID" ] && [ -d "$WORKDIR/.claude" ]; then
            chown -R "${PUID}:${PGID}" "$WORKDIR/.claude"
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
if [ ! -d /data/secrets ]; then
    mkdir -p /data/secrets
    chmod 0700 /data/secrets
fi

AUTH_METHOD="${AUTH_METHOD:-${AUTH_MODE:-none}}"
export AUTH_METHOD

if [ -n "$PUID" ]; then
    exec gosu "${PUID}:${PGID}" env HOME=/home/perchuser /app/perch
fi
exec /app/perch
