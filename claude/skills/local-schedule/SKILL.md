---
name: local-schedule
description: Use when creating, listing, or deleting scheduled messages in the current terminal session. Perch schedules send a line of text directly into the PTY at a specified time, not remote agents.
---

# Perch Schedule

Perch's built-in scheduler injects a line of text into the active PTY at a specified time — exactly as if the user typed it and pressed Enter. Claude Code (running in the same session) will receive and respond to it.

## How it works

Schedules are stored as **JSONL** (one JSON object per line) in:

```
$WORKDIR/.perch/schedules.jsonl
```

Perch watches this file with `fsnotify`. Any time you write to it — add a line, remove a line, or change a field — Perch reloads immediately and logs what changed (`added` / `deleted` / `modified`). **There is no HTTP API.** All schedule management is done by reading and writing this file.

## File format

Each line is a self-contained JSON object:

```jsonl
{"id":"a1b2c3d4","hour":9,"minute":0,"message":"幫我做今天的 daily standup 摘要","repeat":true}
{"id":"e5f6a7b8","hour":18,"minute":30,"message":"提醒我收工","repeat":true,"target":"discord:1492464386219184200"}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier. Auto-generated if omitted — Perch will write it back. |
| `hour` | 0–23 | Server local time (set `TZ` env var if not UTC) |
| `minute` | 0–59 | Server local time |
| `message` | string | Task prompt sent to Claude Code (newline appended automatically) |
| `repeat` | bool | `true` = daily, `false` = run once then removed from file |
| `target` | string | **Required when running inside a Discord session.** Use the value of `$PERCH_SESSION_TARGET`. Omit (or leave empty) for the main web PTY. |

## Session target (Discord sessions)

**Always check `$PERCH_SESSION_TARGET` before writing a schedule.**

- If set (e.g. `discord:1492464386219184200`), include it as `target` so the scheduled message fires back to this Discord channel.
- If empty/unset, omit `target` — the message goes to the main web PTY.

```bash
echo $PERCH_SESSION_TARGET   # e.g. "discord:1492464386219184200"
```

## Key design: separate time from task

**`message` is a prompt sent directly to Claude Code.** Write it as a clear, self-contained *request* — not the answer.

✅ Good: `"請給我一句讓我開心的話"` → Claude generates a fresh response each day  
✅ Good: `"幫我做今天的 daily standup 摘要"` → Claude summarises fresh each morning  
❌ Bad: `"你今天很棒！加油！🌟"` → pre-written answer, same every day  

## Operations

### List schedules

```bash
cat "$WORKDIR/.perch/schedules.jsonl"
```

### Add a schedule

Append a line. `id` is optional — Perch auto-assigns one if missing.

```bash
# Detect session (Discord or main PTY)
TARGET="${PERCH_SESSION_TARGET:-}"

# Build the JSON line
if [ -n "$TARGET" ]; then
  LINE="{\"hour\":9,\"minute\":0,\"message\":\"幫我做今天的 daily standup 摘要\",\"repeat\":true,\"target\":\"$TARGET\"}"
else
  LINE='{"hour":9,"minute":0,"message":"幫我做今天的 daily standup 摘要","repeat":true}'
fi

echo "$LINE" >> "$WORKDIR/.perch/schedules.jsonl"
```

Perch detects the change and logs: `schedule added id=... hour=9 minute=0 ...`

### Delete a schedule

Remove the line with the matching `id`:

```bash
# Remove the job with id "a1b2c3d4"
grep -v '"id":"a1b2c3d4"' "$WORKDIR/.perch/schedules.jsonl" > /tmp/_sched.jsonl \
  && mv /tmp/_sched.jsonl "$WORKDIR/.perch/schedules.jsonl"
```

Perch detects the change and logs: `schedule deleted id=a1b2c3d4 ...`

### Modify a schedule

Edit the file directly (e.g. with `jq` or by rewriting the specific line). Perch logs: `schedule modified id=... hour=NEW ...`

## Full example — Daily standup at 09:00

```bash
TARGET="${PERCH_SESSION_TARGET:-}"

if [ -n "$TARGET" ]; then
  echo "{\"hour\":9,\"minute\":0,\"message\":\"幫我做今天的 daily standup 摘要\",\"repeat\":true,\"target\":\"$TARGET\"}" \
    >> "$WORKDIR/.perch/schedules.jsonl"
else
  echo '{"hour":9,"minute":0,"message":"幫我做今天的 daily standup 摘要","repeat":true}' \
    >> "$WORKDIR/.perch/schedules.jsonl"
fi

# Verify
cat "$WORKDIR/.perch/schedules.jsonl"
```
