#!/bin/sh
set -e

# Merge perch-bundled skills into ~/.claude/skills/ at runtime.
# This runs AFTER volume mounts, so it works whether or not the user
# mounts their own ~/.claude.  We use cp -rn (no-overwrite) so a user
# who already has a skill with the same name keeps theirs.
if [ -d /app/perch-claude/skills ]; then
    mkdir -p /root/.claude/skills
    for skill_dir in /app/perch-claude/skills/*/; do
        skill_name=$(basename "$skill_dir")
        if [ ! -d "/root/.claude/skills/$skill_name" ]; then
            cp -r "$skill_dir" "/root/.claude/skills/$skill_name"
        fi
    done
fi

# Merge perch hooks into ~/.claude/settings.json so IM integration works
# even when the user mounts their own settings file.
if [ -f /app/perch-claude/settings.json ]; then
    node /app/perch-claude/merge-settings.js
fi

exec /app/perch
