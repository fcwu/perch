---
name: browser-automation
description: Headless Chromium browser automation via Playwright MCP inside the perch container. Use when a task requires navigating a website, filling forms, downloading files, or any browser interaction. Includes CAPTCHA retry SOP and Discord human-in-the-loop protocol. Does NOT apply to host (Mac) Chrome — use chrome-cdp skill for that.
---

# Browser Automation

Use the `browser_*` MCP tools provided by `@playwright/mcp` to automate web interactions headlessly inside the container.

## When to Use

- Downloading files that require login or form interaction
- Navigating web UIs to extract data that has no API
- Automating repetitive browser-based tasks

## When NOT to Use

- The target site has a direct API or CLI equivalent (prefer those)
- Bulk/mass scraping that resembles bot activity
- On the host Mac Chrome session — use `chrome-cdp` skill instead
- Sites that explicitly prohibit automation in their ToS (ask Doro first)

## Core MCP Tools

| Tool | Purpose |
|------|---------|
| `browser_navigate` | Open a URL |
| `browser_take_screenshot` | Capture current page (essential for CAPTCHA vision and debugging) |
| `browser_snapshot` | Get accessibility tree (for discovering element selectors) |
| `browser_click` | Click an element |
| `browser_type` | Type text into a field |
| `browser_select_option` | Select a dropdown value |
| `browser_wait_for` | Wait for element or navigation to settle |

## CAPTCHA Handling SOP

1. `browser_take_screenshot` — inspect the CAPTCHA visually
2. Read digits directly from the screenshot using vision
3. `browser_type` the digits into the CAPTCHA field
4. Submit and check for error indicators (error message, page still on login)
5. If error: locate the "重新產生驗證碼" / "reload" link, click it, retry (up to 3 attempts total)
6. After 3 consecutive failures: post current CAPTCHA screenshot to Discord with message:
   > 「我連續 3 次填錯驗證碼，請幫我確認截圖中的數字是什麼？」
7. Pause and wait for Doro's reply, then use the supplied value

## Sensitive Credentials Injection

**Never** read a secret and echo it into conversation. Use shell expansion inside the `text` argument:

```
browser_type(selector="#id-field", text=$(jq -r .id /data/secrets/<site>.json))
browser_type(selector="#birth-field", text=$(jq -r .birth /data/secrets/<site>.json))
```

Narrate as: "已從 `/data/secrets/<site>.json` 填入身分證號" — do NOT print the actual value.

If `/data/secrets/<site>.json` is missing, stop immediately and report:

> 「缺少 `/data/secrets/<site>.json`。請在 container 的 `/data/secrets/` 目錄建立該檔案（chmod 600），格式：`{"id": "A123456789", "birth": "0700101"}`。請勿在 Discord 貼上實際數值。」

## storageState Rules

For sites whose first-time login needs OTP, OAuth, or biometrics:

1. Check if `/data/playwright/state/<site>.json` exists
2. If present: pass `--storage-state=/data/playwright/state/<site>.json` to the browser context
3. If absent: **do not** attempt headed execution or prompt for credentials in Discord. Report:
   > 「需要先在 Mac 端執行 `tests/playwright-login.sh <site>` 完成首次登入並上傳 storageState，再重試。」

## Discord Intervention — When to Stop and Ask

Stop and post a screenshot + question to Discord when:

- CAPTCHA fails 3+ consecutive times
- Page structure doesn't match expected selectors (unexpected page layout)
- Non-image CAPTCHA appears (hCaptcha, Cloudflare Turnstile, reCAPTCHA)
- OTP / SMS verification step appears
- A download event doesn't fire within ~30 seconds

Message template:
```
【瀏覽器自動化需要協助】
任務：<task description>
狀況：<what happened vs. what was expected>
[screenshot]
問題：<specific question for Doro>
```

After Doro replies, continue from the exact point of pause.

## Failure / Retry Limits

| Scenario | Auto-retries | Action on limit |
|----------|-------------|-----------------|
| CAPTCHA wrong answer | 3 | Screenshot → ask Doro |
| Element not found | 2 | Full-page screenshot + a11y tree → ask Doro |
| Download timeout | 2 | Screenshot → describe state → ask Doro |
| Unexpected page | 1 | Screenshot + describe delta → ask Doro |

## Displaying Screenshots to the User

When you call `browser_take_screenshot` (no `filename` parameter), the screenshot is automatically saved to `/tmp/playwright-output/` with a timestamped name. The tool result text contains a link like:

```
- [Screenshot of viewport](../tmp/playwright-output/page-2026-05-09T01-52-04-689Z.png)
```

To display it to the user, emit the absolute path:

```
[image: /tmp/playwright-output/page-2026-05-09T01-52-04-689Z.png]
```

Use the **exact filename** from the tool result (everything after the last `/` in the link). The inline base64 in the tool result is only visible to you — the `[image: ...]` token is required for the image to appear in the user's chat.

Do NOT use the `filename` parameter in `browser_take_screenshot` — it resolves relative to the app directory which is read-only.

## Profile & Session Notes

Chromium uses a temporary profile per MCP session; there is no shared persistent `--user-data-dir`. Avoid launching concurrent browser sessions from different conversations, as they can conflict over the state directory.

## Bot Detection Fallback Ladder

| Level | What | When to escalate |
|-------|------|-----------------|
| 0 | Plain `--headless` | Default |
| 1 | + persistent `--user-data-dir` (auto via profile dir) | Already active |
| 2 | + `--disable-blink-features=AutomationControlled` stealth arg | If site detects headless |
| 3 | Xvfb + headed inside container | If Level 2 still blocked |
| 4 | Mac-side `chrome-cdp` + state sync | If container execution blocked entirely |

Levels 2–4 are not pre-implemented. Escalate only when an actual site triggers the need; post a Discord message before escalating beyond Level 1.
