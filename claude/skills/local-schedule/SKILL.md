---
name: local-schedule
description: Use when creating, listing, or deleting scheduled messages in the current terminal session. Perch schedules send a line of text directly into the PTY at a specified time, not remote agents.
---

# Perch Schedule

Perch's built-in scheduler injects a line of text into the active PTY at a specified time — exactly as if the user typed it and pressed Enter. Claude Code (running in the same session) will receive and respond to it.

## Key design: separate time from task

When the user says something like *"每天早上 9 點幫我做 daily standup"*:

| Part | Value | Goes into |
|------|-------|-----------|
| Time | 09:00 | `hour: 9, minute: 0` |
| Task | 幫我做 daily standup | `message` field |

**`message` is a prompt sent directly to Claude Code.** Write it as a clear, self-contained *request* — not the answer. Claude Code receives it at the scheduled time and generates a fresh response each time.

✅ Good: `"請給我一句讓我開心的話"`  → Claude Code generates a different uplifting message every day  
✅ Good: `"幫我做今天的 daily standup 摘要"` → Claude Code summarises fresh each morning  
❌ Bad: `"你今天很棒！加油！🌟"` → pre-generated answer, same every day  
❌ Bad: `"每天早上9點幫我做 daily standup"` → includes time info, redundant

## Base URL

```bash
BASE="http://localhost${LISTEN_ADDR:-:8443}"
```

## Commands

### List schedules

```bash
curl -s "$BASE/schedule"
```

Response: JSON array of jobs. Empty array `[]` means no schedules.

### Add schedule

```bash
curl -s -X POST "$BASE/schedule" \
  -H "Content-Type: application/json" \
  -d '{"hour": 9, "minute": 0, "message": "幫我做今天的 daily standup 摘要", "repeat": true}'
```

Response (201 Created): the created job with its `id`:
```json
{"id":"a1b2c3d4","hour":9,"minute":0,"message":"幫我做今天的 daily standup 摘要","repeat":true}
```

Check `id` is present — if the response contains an `id` field, the schedule was saved successfully.

| Field | Type | Description |
|-------|------|-------------|
| `hour` | 0–23 | Server local time (set `TZ` env var if not UTC) |
| `minute` | 0–59 | Server local time |
| `message` | string | Task prompt sent to Claude Code (newline appended automatically) |
| `repeat` | bool | `true` = daily, `false` = run once then auto-delete |
| `target` | string | **Required when running inside a Discord session.** Use the value of `$PERCH_SESSION_TARGET`. Omit (or leave empty) for the main web PTY. |

**Important — Discord sessions:** If the env var `PERCH_SESSION_TARGET` is set (it is always set inside a Discord PTY), you **must** include `"target": "$PERCH_SESSION_TARGET"` in the POST body so the scheduled message fires back to this Discord channel, not the main session.

```bash
# Inside a Discord PTY — read the target from the environment
TARGET="${PERCH_SESSION_TARGET}"
curl -s -X POST "$BASE/schedule" \
  -H "Content-Type: application/json" \
  -d "{\"hour\": 9, \"minute\": 0, \"message\": \"幫我做今天的 daily standup 摘要\", \"repeat\": true, \"target\": \"$TARGET\"}"
```

### Delete schedule

```bash
curl -s -X DELETE "$BASE/schedule/<id>"
```

Returns 204 No Content on success.

## Full example — Daily standup at 09:00

```bash
# Detect whether we are inside a Discord session
TARGET="${PERCH_SESSION_TARGET:-}"

# Build the JSON body (include target only when non-empty)
if [ -n "$TARGET" ]; then
  BODY="{\"hour\": 9, \"minute\": 0, \"message\": \"幫我做今天的 daily standup 摘要\", \"repeat\": true, \"target\": \"$TARGET\"}"
else
  BODY='{"hour": 9, "minute": 0, "message": "幫我做今天的 daily standup 摘要", "repeat": true}'
fi

# Create
curl -s -X POST "$BASE/schedule" \
  -H "Content-Type: application/json" \
  -d "$BODY"

# Verify
curl -s "$BASE/schedule"
```
