#!/bin/sh
set -e

# If PUID is set, create a home directory owned by PUID:PGID so the
# non-root claude process has a writable home. perch itself stays root.
if [ -n "$PUID" ]; then
    PGID="${PGID:-$PUID}"
    mkdir -p /home/perchuser
    chown "${PUID}:${PGID}" /home/perchuser
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

        # Merge perch hooks into $WORKDIR/.claude/settings.json only when IM
        # integration is configured, and only into the project-level config so the
        # host-mounted ~/.claude/settings.json is never modified.
        if [ -f /app/perch-claude/settings.json ] && \
           [ -n "${DISCORD_BOT_TOKEN}${TELEGRAM_BOT_TOKEN}" ]; then
            PERCH_MERGE_TARGET="$WORKDIR/.claude/settings.json" \
            node /app/perch-claude/merge-settings.js
        fi

        # Fix ownership of anything created above under .claude/ (mkdir/cp/node all
        # run as root).  Only applies when PUID is set; harmless if .claude/ doesn't
        # exist yet.
        if [ -n "$PUID" ] && [ -d "$WORKDIR/.claude" ]; then
            chown -R "${PUID}:${PGID}" "$WORKDIR/.claude"
        fi
    fi
fi

exec /app/perch
