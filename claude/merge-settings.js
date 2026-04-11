#!/usr/bin/env node
// Merges perch hooks from /app/perch-claude/settings.json into
// /root/.claude/settings.json without overwriting hooks the user
// already has configured.

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

const user = loadJSON(USER_SETTINGS)
const perch = loadJSON(PERCH_SETTINGS)

if (!perch.hooks) process.exit(0)

user.hooks = user.hooks || {}

for (const [event, entries] of Object.entries(perch.hooks)) {
  if (!Array.isArray(entries)) continue
  user.hooks[event] = user.hooks[event] || []
  for (const entry of entries) {
    const cmd = entry.command
    if (!cmd) continue
    const alreadyPresent = user.hooks[event].some(e => e.command === cmd)
    if (!alreadyPresent) {
      user.hooks[event].push(entry)
    }
  }
}

fs.mkdirSync('/root/.claude', { recursive: true })
fs.writeFileSync(USER_SETTINGS, JSON.stringify(user, null, 2) + '\n', { mode: 0o600 })
console.log('perch: merged hooks into', USER_SETTINGS)
