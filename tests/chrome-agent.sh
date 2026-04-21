#!/bin/bash
# Launch a dedicated Chrome instance for agent/test use (port 9223).
# Run once per login; safe to re-run if already running.
#
# Usage:
#   tests/chrome-agent.sh          # start
#   tests/chrome-agent.sh stop     # kill
#
# CDP_PORT_FILE is written to tests/.chrome-agent/DevToolsActivePort
# Set CDP_PORT_FILE to that path so chrome-cdp skill uses this instance.

PORT=9223
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="$SCRIPT_DIR/.chrome-agent"
PORT_FILE="$DATA_DIR/DevToolsActivePort"
LOG="$DATA_DIR/chrome.log"

mkdir -p "$DATA_DIR"

if [[ "${1:-}" == "stop" ]]; then
  pkill -f "remote-debugging-port=$PORT" 2>/dev/null && echo "chrome-agent stopped" || echo "chrome-agent not running"
  rm -f "$PORT_FILE"
  exit 0
fi

# Already running?
if lsof -ti tcp:$PORT &>/dev/null; then
  if [[ ! -f "$PORT_FILE" ]]; then
    WS=$(grep -o '/devtools/browser/[a-z0-9-]*' "$LOG" 2>/dev/null | tail -1)
    [[ -n "$WS" ]] && printf '%s\n%s\n' "$PORT" "$WS" > "$PORT_FILE"
  fi
  echo "chrome-agent already running on port $PORT"
  exit 0
fi

rm -f "$PORT_FILE"

"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --remote-debugging-port=$PORT \
  --user-data-dir="$DATA_DIR" \
  --no-first-run \
  --no-default-browser-check \
  --window-size=1280,800 \
  &>"$LOG" &

for i in $(seq 1 20); do
  WS=$(grep -o '/devtools/browser/[a-z0-9-]*' "$LOG" 2>/dev/null | tail -1)
  if [[ -n "$WS" ]]; then
    printf '%s\n%s\n' "$PORT" "$WS" > "$PORT_FILE"
    echo "chrome-agent started: ws://127.0.0.1:$PORT$WS"
    exit 0
  fi
  sleep 0.5
done

echo "chrome-agent: timeout waiting for DevTools port" >&2
exit 1
