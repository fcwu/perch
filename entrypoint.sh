#!/bin/sh
set -e

# Determine workspace directory (same logic as main.go)
WORKDIR="${CLAUDE_WORKDIR}"
if [ -z "$WORKDIR" ] && [ -d /workspace ]; then
    WORKDIR="/workspace"
fi

if [ -n "$WORKDIR" ]; then
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
fi

exec /app/perch
