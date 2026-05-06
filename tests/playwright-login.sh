#!/usr/bin/env bash
# Bootstrap Playwright storageState for a given site on Mac (headed Chromium).
# Run this once on your Mac when a site needs complex first-time login
# (OAuth, OTP, biometrics) that can't be completed inside the container.
#
# Usage:
#   tests/playwright-login.sh <site>       # login + upload to default remote
#   tests/playwright-login.sh <site> --dry-run  # login only, skip scp
#
# Supported sites (add more at the bottom of the SITES section):
#   google   https://accounts.google.com
#   fubon    https://card.tcbbank.com.tw
#
# After running, you'll see a headed Chrome window. Log in manually, then
# close the window. The script saves storageState.json and copies it to
# the container host at /data/playwright/state/<site>.json.
#
# Requires: Node.js + npx (playwright is launched via npx, no global install needed)
# Remote host config: set PLAYWRIGHT_REMOTE_HOST env var, or add PLAYWRIGHT_REMOTE_HOST
# to your tests/.env.<hostname>.md file as a line: PLAYWRIGHT_REMOTE_HOST: user@host

set -euo pipefail

SITE="${1:-}"
DRY_RUN="${2:-}"

if [ -z "$SITE" ]; then
    echo "Usage: $0 <site> [--dry-run]" >&2
    echo "" >&2
    echo "Supported sites:" >&2
    echo "  google   https://accounts.google.com" >&2
    echo "  fubon    https://card.tcbbank.com.tw" >&2
    exit 1
fi

# --- Site → URL mapping ---
case "$SITE" in
    google)  LOGIN_URL="https://accounts.google.com" ;;
    fubon)   LOGIN_URL="https://card.tcbbank.com.tw" ;;
    *)
        echo "Unknown site: '$SITE'. Add it to the SITES section in this script." >&2
        exit 1
        ;;
esac

# --- Remote host resolution ---
REMOTE_HOST="${PLAYWRIGHT_REMOTE_HOST:-}"
REMOTE_PATH="/data/playwright/state"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOSTNAME_LOWER="$(hostname -s | tr '[:upper:]' '[:lower:]')"
ENV_FILE="$SCRIPT_DIR/.env.${HOSTNAME_LOWER}.md"

if [ -z "$REMOTE_HOST" ] && [ -f "$ENV_FILE" ]; then
    REMOTE_HOST=$(grep -m1 'PLAYWRIGHT_REMOTE_HOST' "$ENV_FILE" \
        | sed 's/.*PLAYWRIGHT_REMOTE_HOST[^:]*:[[:space:]]*//' \
        | tr -d '`"' \
        | xargs 2>/dev/null || true)
fi

if [ -z "$REMOTE_HOST" ] && [ "${DRY_RUN}" != "--dry-run" ]; then
    echo "Warning: PLAYWRIGHT_REMOTE_HOST not set." >&2
    echo "Set it in your environment or add a line to $ENV_FILE:" >&2
    echo "  PLAYWRIGHT_REMOTE_HOST: user@host" >&2
    read -rp "Remote host (user@host, or leave empty to skip upload): " REMOTE_HOST
fi

STATE_FILE="/tmp/playwright-state-${SITE}-$$.json"
HELPER_SCRIPT="/tmp/playwright-login-helper-$$.mjs"

# --- Write a minimal Node.js helper that opens headed Chromium + saves state ---
cat > "$HELPER_SCRIPT" << 'NODEJS_EOF'
import { chromium } from 'playwright';
import { createInterface } from 'readline';
import { writeFileSync } from 'fs';

const [,, url, stateFile] = process.argv;
if (!url || !stateFile) { console.error('Usage: node helper.mjs <url> <stateFile>'); process.exit(1); }

const browser = await chromium.launch({ headless: false });
const context = await browser.newContext();
const page = await context.newPage();
await page.goto(url);

console.log('');
console.log('Browser is open. Complete the login manually.');
console.log('Press Enter here when you are done (logged in and on a stable page).');
console.log('');

const rl = createInterface({ input: process.stdin, output: process.stdout });
await new Promise(resolve => rl.once('line', resolve));
rl.close();

const state = await context.storageState();
writeFileSync(stateFile, JSON.stringify(state, null, 2));
await browser.close();

console.log('storageState saved.');
NODEJS_EOF

trap 'rm -f "$HELPER_SCRIPT" "$STATE_FILE"' EXIT

echo "=== Playwright Login Bootstrap ==="
echo "Site:    $SITE"
echo "URL:     $LOGIN_URL"
echo "Output:  $STATE_FILE"
echo ""

# Run the helper — installs playwright on first use via npx
npx --yes playwright@latest node "$HELPER_SCRIPT" "$LOGIN_URL" "$STATE_FILE"

if [ ! -s "$STATE_FILE" ]; then
    echo "Error: storageState file is empty or missing." >&2
    exit 1
fi

echo "storageState saved ($(wc -c < "$STATE_FILE") bytes)"

if [ "${DRY_RUN}" = "--dry-run" ]; then
    echo "Dry run: skipping upload. File is at $STATE_FILE (will be cleaned on exit)."
    # Keep file for inspection — remove trap
    trap - EXIT
    echo "File kept at: $STATE_FILE"
    exit 0
fi

if [ -n "$REMOTE_HOST" ]; then
    echo "Uploading to ${REMOTE_HOST}:${REMOTE_PATH}/${SITE}.json ..."
    ssh "$REMOTE_HOST" "mkdir -p ${REMOTE_PATH}"
    scp "$STATE_FILE" "${REMOTE_HOST}:${REMOTE_PATH}/${SITE}.json"
    echo "Done. storageState is now available in the container at ${REMOTE_PATH}/${SITE}.json"
else
    echo "No remote host configured — storageState not uploaded."
    echo "Copy manually: scp $STATE_FILE user@host:${REMOTE_PATH}/${SITE}.json"
fi
