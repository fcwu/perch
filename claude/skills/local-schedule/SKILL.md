---
name: local-schedule
description: Use when creating, listing, or deleting scheduled messages in the current terminal session. Perch schedules send a line of text directly into the PTY at a specified time, not remote agents.
---

# Perch Schedule

Perch's built-in scheduler sends a line of text into the current terminal session at a specified time (cron-style). This is **not** a remote agent scheduler — the message is typed directly into the active PTY.

## Base URL

```bash
BASE="http://localhost${LISTEN_ADDR:-:8443}"
```

`LISTEN_ADDR` is set in the environment (e.g. `:8080`, `:8443`).

## Commands

### List schedules

```bash
curl -s "http://localhost${LISTEN_ADDR:-:8443}/schedule"
```

### Add schedule

```bash
curl -s -X POST "http://localhost${LISTEN_ADDR:-:8443}/schedule" \
  -H "Content-Type: application/json" \
  -d '{"hour": 9, "minute": 0, "message": "your message here", "repeat": true}'
```

| Field | Type | Description |
|-------|------|-------------|
| `hour` | 0–23 | Server local time |
| `minute` | 0–59 | Server local time |
| `message` | string | Text sent to the terminal (newline appended automatically) |
| `repeat` | bool | `true` = daily, `false` = run once then auto-delete |

### Delete schedule

```bash
curl -s -X DELETE "http://localhost${LISTEN_ADDR:-:8443}/schedule/<id>"
```

Get `<id>` from the list response.

## Example — Daily standup at 09:00

```bash
curl -s -X POST "http://localhost${LISTEN_ADDR:-:8443}/schedule" \
  -H "Content-Type: application/json" \
  -d '{
    "hour": 9,
    "minute": 0,
    "message": "幫我做今天的 daily standup 摘要",
    "repeat": true
  }'
```
