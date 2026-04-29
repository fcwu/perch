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

# Locate a Chrome/Chromium binary across macOS and Linux (incl. WSL).
CHROME_BIN=""
CHROME_EXTRA_ARGS=()
for candidate in \
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  "/Applications/Chromium.app/Contents/MacOS/Chromium" \
  "$(command -v google-chrome 2>/dev/null)" \
  "$(command -v google-chrome-stable 2>/dev/null)" \
  "$(command -v chromium-browser 2>/dev/null)" \
  "$(command -v chromium 2>/dev/null)"; do
  if [[ -n "$candidate" && -x "$candidate" ]]; then
    CHROME_BIN="$candidate"
    break
  fi
done

if [[ -z "$CHROME_BIN" ]]; then
  echo "chrome-agent: no Chrome/Chromium binary found (looked for google-chrome, chromium, macOS bundles)" >&2
  echo "  install: 'sudo apt-get install -y google-chrome-stable' or 'sudo apt-get install -y chromium-browser'" >&2
  exit 1
fi

# Headed vs headless on Linux:
#   - Headed needs a display (WSLg / X server / Wayland). Required when the user
#     needs to interactively log in (Discord, GitLab) — the session then persists
#     in DATA_DIR and later runs (headed or headless) reuse it.
#   - Headless skips the UI; works for tests that don't need new login flows but
#     CANNOT do an interactive login itself.
# Default: headed when a display is detected; otherwise refuse to start and tell
# the user to either enable a display (WSLg / X server) or set CHROME_HEADLESS=1
# explicitly. Silent fallback to headless was removed because it makes login-
# dependent tests fail in surprising ways.
if [[ "$(uname -s)" == "Linux" ]]; then
  HAS_DISPLAY=0
  [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]] && HAS_DISPLAY=1
  case "${CHROME_HEADLESS:-auto}" in
    1|true|yes)
      CHROME_EXTRA_ARGS+=(--headless=new --no-sandbox --disable-gpu)
      ;;
    0|false|no)
      if [[ "$HAS_DISPLAY" == 0 ]]; then
        echo "chrome-agent: CHROME_HEADLESS=0 requested but no DISPLAY/WAYLAND_DISPLAY set" >&2
        echo "  fix: enable WSLg / launch an X server, then re-run; or unset CHROME_HEADLESS to use auto mode" >&2
        exit 1
      fi
      ;;
    auto)
      if [[ "$HAS_DISPLAY" == 0 ]]; then
        echo "chrome-agent: no display detected (DISPLAY/WAYLAND_DISPLAY empty)" >&2
        echo "  for tests that need interactive login (Discord, GitLab) — enable WSLg or X server" >&2
        echo "  for tests that DON'T need login — re-run with: CHROME_HEADLESS=1 tests/chrome-agent.sh" >&2
        echo "  to seed DATA_DIR from another machine — copy tests/.chrome-agent/ over after logging in there" >&2
        exit 1
      fi
      ;;
    *)
      echo "chrome-agent: invalid CHROME_HEADLESS=${CHROME_HEADLESS} (use 1 / 0 / unset)" >&2
      exit 1
      ;;
  esac
fi

"$CHROME_BIN" \
  --remote-debugging-port=$PORT \
  --user-data-dir="$DATA_DIR" \
  --no-first-run \
  --no-default-browser-check \
  --window-size=1280,800 \
  "${CHROME_EXTRA_ARGS[@]}" \
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
