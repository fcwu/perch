#!/usr/bin/env node
// Merges perch hooks from /app/perch-claude/settings.json into
// /root/.claude/settings.json without overwriting hooks the user
// already has configured.
//
// Hook format (Claude Code ≥ 1.x):
// { "hooks": { "PreToolUse": [{ "matcher": "", "hooks": [{ "type": "command", "command": "..." }] }] } }

const fs = require('fs')

const USER_SETTINGS = '/root/.claude/settings.json'
const PERCH_SETTINGS = '/app/perch-claude/settings.json'

function loadJSON(path) {
  try {
    return JSON.parse(fs.readFileSync(path, 'utf8'))
  } catch {
    return {}
  }
}

// Collect all "command" strings already present in a hook-event array.
function existingCommands(eventEntries) {
  const cmds = new Set()
  for (const entry of eventEntries) {
    if (Array.isArray(entry.hooks)) {
      for (const h of entry.hooks) {
        if (h.command) cmds.add(h.command)
      }
    }
    // legacy flat format
    if (entry.command) cmds.add(entry.command)
  }
  return cmds
}

const user = loadJSON(USER_SETTINGS)
const perch = loadJSON(PERCH_SETTINGS)

if (!perch.hooks) process.exit(0)

user.hooks = user.hooks || {}

for (const [event, entries] of Object.entries(perch.hooks)) {
  if (!Array.isArray(entries)) continue
  user.hooks[event] = user.hooks[event] || []

  const existing = existingCommands(user.hooks[event])

  for (const entry of entries) {
    if (!Array.isArray(entry.hooks)) continue
    const newHooks = entry.hooks.filter(h => h.command && !existing.has(h.command))
    if (newHooks.length === 0) continue
    user.hooks[event].push({ matcher: entry.matcher ?? '', hooks: newHooks })
    newHooks.forEach(h => existing.add(h.command))
  }
}

fs.mkdirSync('/root/.claude', { recursive: true })
fs.writeFileSync(USER_SETTINGS, JSON.stringify(user, null, 2) + '\n', { mode: 0o600 })
console.log('perch: merged hooks into', USER_SETTINGS)
