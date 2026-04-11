#!/usr/bin/env node
// Merges perch hooks from /app/perch-claude/settings.json into the
// target settings file (PERCH_MERGE_TARGET env var, defaults to
// /root/.claude/settings.json) without overwriting existing hooks.
//
// Hook format (Claude Code ≥ 1.x):
// { "hooks": { "PreToolUse": [{ "matcher": "", "hooks": [{ "type": "command", "command": "..." }] }] } }

const fs = require('fs')
const path = require('path')

const TARGET = process.env.PERCH_MERGE_TARGET || '/root/.claude/settings.json'
const PERCH_SETTINGS = '/app/perch-claude/settings.json'

function loadJSON(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'))
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

const user = loadJSON(TARGET)
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

fs.mkdirSync(path.dirname(TARGET), { recursive: true })
fs.writeFileSync(TARGET, JSON.stringify(user, null, 2) + '\n', { mode: 0o600 })
console.log('perch: merged hooks into', TARGET)
