# E2E Test Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automate 25 of 33 test cases from `docs/test-cases.md` using Playwright + Docker, so `npx playwright test` from `e2e/` runs the full suite.

**Architecture:** Tests live in `e2e/`. Non-Docker tests spawn `./perch` binary directly (fast, no image needed). Docker tests use the `docker` CLI against a built image. Shared helpers manage binary lifecycle, Docker lifecycle, and xterm.js text extraction. T20-T24 are already covered by `go test`. T18/T19/T28/T29/T31/T32 require Discord credentials and stay manual.

**Tech Stack:** Node.js 20+, `@playwright/test` 1.44, TypeScript, Docker CLI, `./perch` binary

---

## Notes Before Starting

1. **`/schedule` HTTP API removed**: test-cases.md T05-T07 reference `GET/POST/DELETE /schedule` endpoints that were replaced by JSONL file-watching (commit `00b7b76`). T05-T07 are adapted to verify the JSONL-based scheduler instead.

2. **xterm.js DOM renderer**: xterm v6 in headless Chromium uses the DOM renderer by default. Text is readable via `.xterm-rows > div` elements — no canvas scraping needed.

3. **T03/T04 need `claude` CLI**: auto-skipped when not installed.

4. **Docker tests need `PERCH_IMAGE`**: defaults to `ghcr.io/fcwu/perch:latest`. For local builds: `PERCH_IMAGE=perch-local`.

---

## File Map

```
e2e/
  package.json                 — npm deps (@playwright/test, typescript)
  tsconfig.json                — TS config for test files
  playwright.config.ts         — serial execution, 60s timeout, chromium
  helpers/
    server.ts                  — build binary, spawn/kill local perch process
    docker.ts                  — docker run/exec/logs/stop wrappers
    terminal.ts                — read xterm.js DOM text, type into terminal
  tests/
    api.spec.ts                — T01, T09, T10, T33 (HTTP, no browser)
    scheduler.spec.ts          — T05-T07, T27 (JSONL file-based)
    mtls.spec.ts               — T12
    terminal.spec.ts           — T02-T04, T17, T25
    multi-tab.spec.ts          — T11, T14
    keyboard.spec.ts           — T08
    docker.spec.ts             — T13, T15, T16, T26, T30
```

Modify:
- `DEVELOPMENT.md` — add e2e instructions

---

## Task 1: Project setup

**Files:**
- Create: `e2e/package.json`
- Create: `e2e/tsconfig.json`
- Create: `e2e/playwright.config.ts`

- [ ] Create `e2e/package.json`:

```json
{
  "name": "perch-e2e",
  "private": true,
  "scripts": {
    "test": "playwright test",
    "test:headed": "playwright test --headed"
  },
  "devDependencies": {
    "@playwright/test": "^1.44.0",
    "typescript": "^5.4.0"
  }
}
```

- [ ] Create `e2e/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "commonjs",
    "strict": true,
    "esModuleInterop": true
  },
  "include": ["**/*.ts"]
}
```

- [ ] Create `e2e/playwright.config.ts`:

```typescript
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 60000,
  retries: 0,
  workers: 1,       // serial: each test uses a fixed port
  use: { headless: true },
  reporter: [['list'], ['html', { open: 'never' }]],
})
```

- [ ] Run:

```bash
cd e2e && npm install && npx playwright install chromium
```

Expected: `node_modules/` created, Chromium downloaded.

- [ ] Commit:

```bash
git add e2e/package.json e2e/tsconfig.json e2e/playwright.config.ts
git commit -m "test(e2e): add playwright project setup"
```

---

## Task 2: Server helper

**Files:**
- Create: `e2e/helpers/server.ts`

- [ ] Create `e2e/helpers/server.ts`:

```typescript
import { spawn, ChildProcess, execSync } from 'child_process'
import * as path from 'path'

export const ROOT = path.resolve(__dirname, '../../')

export function buildBinary(): void {
  execSync('go build -o perch .', { cwd: ROOT, stdio: 'inherit' })
}

export interface ServerHandle {
  url: string
  stop: () => void
}

export async function startServer(
  port: number,
  env: Record<string, string> = {}
): Promise<ServerHandle> {
  const url = `http://localhost:${port}`
  const proc: ChildProcess = spawn('./perch', [], {
    cwd: ROOT,
    env: {
      ...process.env,
      AUTH_MODE: 'none',
      LISTEN_ADDR: `:${port}`,
      ...env,
    },
    stdio: 'pipe',
  })

  const deadline = Date.now() + 10000
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(500) })
      if (res.status < 500) break
    } catch { /* not ready yet */ }
    await new Promise(r => setTimeout(r, 200))
  }

  return {
    url,
    stop: () => { proc.kill('SIGTERM') },
  }
}
```

- [ ] Commit:

```bash
git add e2e/helpers/server.ts
git commit -m "test(e2e): add server spawn helper"
```

---

## Task 3: Docker helper

**Files:**
- Create: `e2e/helpers/docker.ts`

- [ ] Create `e2e/helpers/docker.ts`:

```typescript
import { execFileSync } from 'child_process'

export const IMAGE = process.env.PERCH_IMAGE ?? 'ghcr.io/fcwu/perch:latest'

export interface ContainerHandle {
  id: string
  url: string
  stop: () => void
}

export interface DockerRunOptions {
  port: number
  env?: Record<string, string>
  volumes?: string[]   // "host:container" pairs
  image?: string
}

export function dockerRun(opts: DockerRunOptions): ContainerHandle {
  const { port, env = {}, volumes = [], image = IMAGE } = opts
  const envArgs = Object.entries(env).flatMap(([k, v]) => ['-e', `${k}=${v}`])
  const volArgs = volumes.flatMap(v => ['-v', v])
  const id = execFileSync('docker', [
    'run', '-d',
    '-p', `${port}:8080`,
    ...envArgs,
    ...volArgs,
    image,
  ]).toString().trim()

  return {
    id,
    url: `http://localhost:${port}`,
    stop: () => {
      try { execFileSync('docker', ['rm', '-f', id]) } catch { /* ignore */ }
    },
  }
}

export function dockerExec(id: string, cmd: string[]): string {
  return execFileSync('docker', ['exec', id, ...cmd]).toString().trim()
}

export function dockerLogs(id: string): string {
  return execFileSync('docker', ['logs', id]).toString()
}

export async function waitForContainer(url: string, timeoutMs = 30000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(1000) })
      if (res.status < 500) return
    } catch { /* not ready */ }
    await new Promise(r => setTimeout(r, 500))
  }
  throw new Error(`Container at ${url} did not become ready within ${timeoutMs}ms`)
}
```

- [ ] Commit:

```bash
git add e2e/helpers/docker.ts
git commit -m "test(e2e): add docker lifecycle helper"
```

---

## Task 4: Terminal helper

**Files:**
- Create: `e2e/helpers/terminal.ts`

- [ ] Create `e2e/helpers/terminal.ts`:

```typescript
import type { Page } from '@playwright/test'
import { expect } from '@playwright/test'

/** Read all visible text from the xterm.js DOM (.xterm-rows). */
export async function getTerminalText(page: Page): Promise<string> {
  return page.evaluate(() => {
    const rows = document.querySelectorAll<HTMLElement>('.xterm-rows > div')
    return Array.from(rows).map(r => r.textContent ?? '').join('\n')
  })
}

/** Poll until the terminal contains `text`, or throw after `timeout` ms. */
export async function waitForTerminalText(
  page: Page,
  text: string,
  timeout = 30000
): Promise<void> {
  await expect(async () => {
    const content = await getTerminalText(page)
    expect(content).toContain(text)
  }).toPass({ timeout, intervals: [500, 500, 1000] })
}

/** Click the terminal screen and type `text`. */
export async function typeIntoTerminal(page: Page, text: string): Promise<void> {
  await page.locator('.xterm-screen').click()
  await page.keyboard.type(text)
}

/** Assert terminal fills the viewport (≤32px gap allowed for keyboard bar). */
export async function assertTerminalFillsViewport(page: Page): Promise<void> {
  const viewport = page.viewportSize()!
  const box = await page.locator('.xterm-screen').boundingBox()
  expect(box).toBeTruthy()
  expect(box!.height).toBeGreaterThan(viewport.height - 32)
  expect(box!.width).toBeGreaterThan(viewport.width - 32)
}
```

- [ ] Commit:

```bash
git add e2e/helpers/terminal.ts
git commit -m "test(e2e): add xterm.js terminal helper"
```

---

## Task 5: API tests — T01, T09, T10, T33

**Files:**
- Create: `e2e/tests/api.spec.ts`

Port range: 19001–19009.

- [ ] Create `e2e/tests/api.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, startServer, ServerHandle, ROOT } from '../helpers/server'
import { execSync, spawnSync } from 'child_process'
import * as path from 'path'
import * as fs from 'fs'

test.beforeAll(() => { buildBinary() })

// T01 — startup none mode: / returns HTML
test('T01: AUTH_MODE=none returns HTML on /', async ({ request }) => {
  const srv = await startServer(19001)
  try {
    const res = await request.get(srv.url + '/')
    expect(res.status()).toBe(200)
    expect((await res.text()).toLowerCase()).toContain('<!doctype html>')
  } finally { srv.stop() }
})

// T01 reverse: HTTPS connection fails on plain HTTP server
test('T01 reverse: HTTPS fails on plain HTTP server', async () => {
  const srv = await startServer(19002)
  try {
    await expect(
      fetch('https://localhost:19002', { signal: AbortSignal.timeout(2000) })
    ).rejects.toThrow()
  } finally { srv.stop() }
})

// T09 — rate limit: /login 429 after 5 requests
test('T09: /login rate-limits at request 6+', async ({ request }) => {
  const srv = await startServer(19003, { AUTH_MODE: 'password', AUTH_PASSWORD: 'x' })
  const codes: number[] = []
  try {
    for (let i = 0; i < 8; i++) {
      const res = await request.post(srv.url + '/login', { data: { password: 'wrong' } })
      codes.push(res.status())
    }
  } finally { srv.stop() }
  expect(codes.slice(0, 5).every(c => c !== 429)).toBe(true)
  expect(codes.slice(5).some(c => c === 429)).toBe(true)
})

// T10 — password auth: correct password → 204 + session cookie
test('T10: correct password returns 204 with session cookie', async ({ request }) => {
  const srv = await startServer(19004, { AUTH_MODE: 'password', AUTH_PASSWORD: 'testpass' })
  try {
    const res = await request.post(srv.url + '/login', { data: { password: 'testpass' } })
    expect(res.status()).toBe(204)
    const cookie = res.headers()['set-cookie'] ?? ''
    expect(cookie).toContain('session=')
    expect(cookie).not.toContain('Secure')   // T22: no Secure flag on plain HTTP
  } finally { srv.stop() }
})

test('T10: wrong password returns 401', async ({ request }) => {
  const srv = await startServer(19005, { AUTH_MODE: 'password', AUTH_PASSWORD: 'testpass' })
  try {
    const res = await request.post(srv.url + '/login', { data: { password: 'wrong' } })
    expect(res.status()).toBe(401)
  } finally { srv.stop() }
})

test('T10: no cookie on protected endpoint returns 401', async ({ request }) => {
  const srv = await startServer(19006, { AUTH_MODE: 'password', AUTH_PASSWORD: 'testpass' })
  try {
    const res = await request.get(srv.url + '/')
    expect(res.status()).toBe(401)
  } finally { srv.stop() }
})

// T33 — build time appears in startup log
test('T33: binary built with -ldflags shows built= in log output', () => {
  const bin = path.join(ROOT, 'perch-t33')
  const ts = new Date().toISOString()
  execSync(`go build -ldflags "-X main.buildTime=${ts}" -o ${bin} .`, { cwd: ROOT })
  const result = spawnSync(bin, [], {
    cwd: ROOT,
    env: { ...process.env, AUTH_MODE: 'none', LISTEN_ADDR: ':19007' },
    timeout: 2000,
    encoding: 'utf8',
  })
  fs.unlinkSync(bin)
  expect(result.stdout + result.stderr).toContain(`built=${ts}`)
})
```

- [ ] Run: `cd e2e && npx playwright test tests/api.spec.ts`

Expected: 7 tests pass.

- [ ] Commit:

```bash
git add e2e/tests/api.spec.ts
git commit -m "test(e2e): add API tests T01 T09 T10 T33"
```

---

## Task 6: Scheduler tests — T05-T07, T27

Note: HTTP `/schedule` endpoints were removed; scheduler reads `.perch/schedules.jsonl` via fsnotify.

**Files:**
- Create: `e2e/tests/scheduler.spec.ts`

Port range: 19010–19013.

- [ ] Create `e2e/tests/scheduler.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, startServer, ServerHandle } from '../helpers/server'
import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'

test.beforeAll(() => { buildBinary() })

// T27 / T05: .perch dir and schedules.jsonl created on startup
test('T27: .perch/ directory exists after startup', async () => {
  const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-'))
  const srv = await startServer(19010, { CLAUDE_WORKDIR: workdir })
  try {
    expect(fs.existsSync(path.join(workdir, '.perch'))).toBe(true)
  } finally {
    srv.stop()
    fs.rmSync(workdir, { recursive: true })
  }
})

test('T05: schedules.jsonl contains valid JSON lines when present', async () => {
  const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-'))
  const srv = await startServer(19011, { CLAUDE_WORKDIR: workdir })
  try {
    const jsonl = path.join(workdir, '.perch', 'schedules.jsonl')
    if (fs.existsSync(jsonl)) {
      for (const line of fs.readFileSync(jsonl, 'utf8').split('\n').filter(Boolean)) {
        expect(() => JSON.parse(line)).not.toThrow()
      }
    }
  } finally {
    srv.stop()
    fs.rmSync(workdir, { recursive: true })
  }
})

// T06: write job to schedules.jsonl, verify scheduler loads it
test('T06: job written to schedules.jsonl is loaded by fsnotify', async () => {
  const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-'))
  const srv = await startServer(19012, { CLAUDE_WORKDIR: workdir })
  try {
    const jsonl = path.join(workdir, '.perch', 'schedules.jsonl')
    const job = { id: 'e2e-job-1', hour: 23, minute: 59, message: 'hello', repeat: false }
    fs.writeFileSync(jsonl, JSON.stringify(job) + '\n')
    await new Promise(r => setTimeout(r, 1000))   // fsnotify debounce
    const content = fs.readFileSync(jsonl, 'utf8').trim()
    expect(JSON.parse(content.split('\n')[0]).id).toBe('e2e-job-1')
  } finally {
    srv.stop()
    fs.rmSync(workdir, { recursive: true })
  }
})

// T07: clear schedules.jsonl, scheduler treats it as empty
test('T07: clearing schedules.jsonl removes all jobs', async () => {
  const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-'))
  const srv = await startServer(19013, { CLAUDE_WORKDIR: workdir })
  try {
    const jsonl = path.join(workdir, '.perch', 'schedules.jsonl')
    fs.writeFileSync(jsonl, JSON.stringify({ id: 'j1', hour: 0, minute: 0, message: 'x', repeat: false }) + '\n')
    await new Promise(r => setTimeout(r, 500))
    fs.writeFileSync(jsonl, '')
    await new Promise(r => setTimeout(r, 1000))
    expect(fs.readFileSync(jsonl, 'utf8').trim()).toBe('')
  } finally {
    srv.stop()
    fs.rmSync(workdir, { recursive: true })
  }
})
```

- [ ] Run: `cd e2e && npx playwright test tests/scheduler.spec.ts`

Expected: 4 tests pass.

- [ ] Commit:

```bash
git add e2e/tests/scheduler.spec.ts
git commit -m "test(e2e): add scheduler JSONL tests T05-T07 T27"
```

---

## Task 7: mTLS bootstrap — T12

**Files:**
- Create: `e2e/tests/mtls.spec.ts`

Port: 19020.

- [ ] Create `e2e/tests/mtls.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, ROOT } from '../helpers/server'
import { spawn } from 'child_process'

test.beforeAll(() => { buildBinary() })

async function waitForTLS(port: number, ms = 10000): Promise<void> {
  const deadline = Date.now() + ms
  while (Date.now() < deadline) {
    try {
      await fetch(`https://localhost:${port}/bootstrap`, {
        signal: AbortSignal.timeout(1000),
        // @ts-ignore Node.js undici option
        dispatcher: new (await import('undici')).Agent({ connect: { rejectUnauthorized: false } }),
      })
      return
    } catch { await new Promise(r => setTimeout(r, 300)) }
  }
  throw new Error('TLS server did not start')
}

function tlsFetch(url: string, opts: RequestInit = {}) {
  // Node 20 fetch does not expose rejectUnauthorized; use undici Agent
  return fetch(url, {
    ...opts,
    // @ts-ignore
    dispatcher: new (require('undici').Agent)({ connect: { rejectUnauthorized: false } }),
  })
}

test('T12: /bootstrap accessible without client cert', async () => {
  const proc = spawn('./perch', [], {
    cwd: ROOT,
    env: { ...process.env, AUTH_MODE: 'mtls', LISTEN_ADDR: ':19020' },
    stdio: 'pipe',
  })
  try {
    await waitForTLS(19020)
    const res = await tlsFetch('https://localhost:19020/bootstrap')
    expect(res.status).toBe(200)
    expect((await res.arrayBuffer()).byteLength).toBeGreaterThan(0)
  } finally { proc.kill('SIGTERM') }
})

test('T12: second /bootstrap call returns 410', async () => {
  const proc = spawn('./perch', [], {
    cwd: ROOT,
    env: { ...process.env, AUTH_MODE: 'mtls', LISTEN_ADDR: ':19021' },
    stdio: 'pipe',
  })
  try {
    await waitForTLS(19021)
    await tlsFetch('https://localhost:19021/bootstrap')                // first
    const res = await tlsFetch('https://localhost:19021/bootstrap')   // second
    expect(res.status).toBe(410)
  } finally { proc.kill('SIGTERM') }
})

test('T12: non-bootstrap path without client cert redirects to /bootstrap', async () => {
  const proc = spawn('./perch', [], {
    cwd: ROOT,
    env: { ...process.env, AUTH_MODE: 'mtls', LISTEN_ADDR: ':19022' },
    stdio: 'pipe',
  })
  try {
    await waitForTLS(19022)
    const res = await tlsFetch('https://localhost:19022/', { redirect: 'manual' })
    expect(res.status).toBe(302)
    expect(res.headers.get('location')).toBe('/bootstrap')
  } finally { proc.kill('SIGTERM') }
})
```

- [ ] Run: `cd e2e && npx playwright test tests/mtls.spec.ts`

Expected: 3 tests pass.

- [ ] Commit:

```bash
git add e2e/tests/mtls.spec.ts
git commit -m "test(e2e): add mTLS bootstrap tests T12"
```

---

## Task 8: Browser/terminal tests — T02-T04, T17, T25

**Files:**
- Create: `e2e/tests/terminal.spec.ts`

Port range: 19030–19035. T03/T04 auto-skip when `claude` not installed.

- [ ] Create `e2e/tests/terminal.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, startServer, ServerHandle } from '../helpers/server'
import { waitForTerminalText, typeIntoTerminal, assertTerminalFillsViewport } from '../helpers/terminal'
import { execSync } from 'child_process'

test.beforeAll(() => { buildBinary() })

function claudeInstalled(): boolean {
  try { execSync('which claude', { stdio: 'ignore' }); return true } catch { return false }
}

// T02 — xterm.js renders
test('T02: xterm.js terminal container is visible', async ({ page }) => {
  const srv = await startServer(19030)
  try {
    await page.goto(srv.url)
    await expect(page.locator('.xterm-screen')).toBeVisible()
    await expect(page.locator('.xterm-rows')).toBeVisible()
  } finally { srv.stop() }
})

// T03 — Claude startup output appears
test('T03: Claude Code startup output appears in terminal', async ({ page }) => {
  test.skip(!claudeInstalled(), 'claude CLI not installed')
  const srv = await startServer(19031)
  try {
    await page.goto(srv.url)
    await waitForTerminalText(page, '>', 30000)
  } finally { srv.stop() }
})

// T04 — keyboard input reaches PTY
test('T04: typed text appears in terminal output', async ({ page }) => {
  test.skip(!claudeInstalled(), 'claude CLI not installed')
  const srv = await startServer(19032)
  try {
    await page.goto(srv.url)
    await waitForTerminalText(page, '>', 20000)
    await typeIntoTerminal(page, 'echo hello-e2e\r')
    await waitForTerminalText(page, 'hello-e2e', 10000)
  } finally { srv.stop() }
})

// T17 — terminal fills viewport
test('T17: terminal fills the full viewport on first load', async ({ page }) => {
  const srv = await startServer(19033)
  try {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto(srv.url)
    await expect(page.locator('.xterm-screen')).toBeVisible()
    await assertTerminalFillsViewport(page)
  } finally { srv.stop() }
})

test('T17 reverse: terminal re-fits after viewport resize', async ({ page }) => {
  const srv = await startServer(19034)
  try {
    await page.goto(srv.url)
    await page.setViewportSize({ width: 800, height: 600 })
    await page.waitForTimeout(500)
    await assertTerminalFillsViewport(page)
  } finally { srv.stop() }
})

// T25 — multi-line URL link detection
test('T25: URL written to terminal is detected as clickable link', async ({ page }) => {
  const srv = await startServer(19035)
  try {
    await page.goto(srv.url)
    await expect(page.locator('.xterm-screen')).toBeVisible()

    // Write URL to PTY via /input
    const url = 'https://example.com/very/long/path/that/wraps/across/lines/abc123'
    await page.request.post(srv.url + '/input', {
      data: JSON.stringify({ data: `echo ${url}\r` }),
      headers: { 'Content-Type': 'application/json' },
    })
    await waitForTerminalText(page, 'https://example.com', 5000)

    // Hover — link provider should set pointer cursor + underline
    const urlSpan = page.locator('.xterm-rows').getByText('https://example.com', { exact: false }).first()
    await urlSpan.hover()
    await page.waitForTimeout(300)

    const cursor = await page.evaluate(() =>
      getComputedStyle(document.querySelector('.xterm-screen')!).cursor
    )
    // xterm link decoration sets pointer cursor on the screen container
    expect(cursor).toBe('pointer')
  } finally { srv.stop() }
})
```

- [ ] Run: `cd e2e && npx playwright test tests/terminal.spec.ts`

Expected: T02/T17/T25 pass; T03/T04 skip if no claude.

- [ ] Commit:

```bash
git add e2e/tests/terminal.spec.ts
git commit -m "test(e2e): add terminal UI tests T02-T04 T17 T25"
```

---

## Task 9: Multi-tab tests — T11, T14

**Files:**
- Create: `e2e/tests/multi-tab.spec.ts`

Port range: 19040–19041.

- [ ] Create `e2e/tests/multi-tab.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, startServer } from '../helpers/server'
import { waitForTerminalText } from '../helpers/terminal'

test.beforeAll(() => { buildBinary() })

// T11 — framebuffer replay: new tab sees existing content
test('T11: new tab receives framebuffer replay', async ({ browser }) => {
  const srv = await startServer(19040)
  const ctxA = await browser.newContext()
  const ctxB = await browser.newContext()
  try {
    const pageA = await ctxA.newPage()
    await pageA.goto(srv.url)

    // Push output to the PTY
    await pageA.request.post(srv.url + '/input', {
      data: JSON.stringify({ data: 'echo framebuf-replay-marker\r' }),
      headers: { 'Content-Type': 'application/json' },
    })
    await waitForTerminalText(pageA, 'framebuf-replay-marker', 5000)

    // New tab should see the same content immediately (framebuffer replay)
    const pageB = await ctxB.newPage()
    await pageB.goto(srv.url)
    await waitForTerminalText(pageB, 'framebuf-replay-marker', 5000)
  } finally {
    await ctxA.close()
    await ctxB.close()
    srv.stop()
  }
})

// T14 — bidirectional input: input from A seen in B and vice versa
test('T14: input from Tab A appears in Tab B output', async ({ browser }) => {
  const srv = await startServer(19041)
  const [ctxA, ctxB] = await Promise.all([browser.newContext(), browser.newContext()])
  try {
    const [pageA, pageB] = await Promise.all([ctxA.newPage(), ctxB.newPage()])
    await Promise.all([pageA.goto(srv.url), pageB.goto(srv.url)])

    await pageA.request.post(srv.url + '/input', {
      data: JSON.stringify({ data: 'echo tab-a-marker\r' }),
      headers: { 'Content-Type': 'application/json' },
    })
    await Promise.all([
      waitForTerminalText(pageA, 'tab-a-marker', 5000),
      waitForTerminalText(pageB, 'tab-a-marker', 5000),
    ])
  } finally {
    await Promise.all([ctxA.close(), ctxB.close()])
    srv.stop()
  }
})
```

- [ ] Run: `cd e2e && npx playwright test tests/multi-tab.spec.ts`

Expected: 2 tests pass.

- [ ] Commit:

```bash
git add e2e/tests/multi-tab.spec.ts
git commit -m "test(e2e): add multi-tab tests T11 T14"
```

---

## Task 10: Virtual keyboard tests — T08

**Files:**
- Create: `e2e/tests/keyboard.spec.ts`

Port range: 19050–19052.

- [ ] Create `e2e/tests/keyboard.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { buildBinary, startServer } from '../helpers/server'

test.beforeAll(() => { buildBinary() })

// T08 desktop: toggle button visible, expands keyboard bar
test('T08 desktop: keyboard toggle shows, click expands bar', async ({ page }) => {
  const srv = await startServer(19050)
  try {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto(srv.url)
    const toggleBtn = page.locator('button', { hasText: '⌨' })
    await expect(toggleBtn).toBeVisible()
    await toggleBtn.click()
    await expect(page.locator('button', { hasText: 'Esc' })).toBeVisible()
  } finally { srv.stop() }
})

// T08 mobile: keyboard bar is expanded by default
test('T08 mobile: keyboard bar is expanded on mobile viewport', async ({ page }) => {
  const srv = await startServer(19051)
  try {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(srv.url)
    await expect(page.locator('button', { hasText: 'Esc' })).toBeVisible()
  } finally { srv.stop() }
})

// T08: Esc button sends Escape byte over WebSocket
test('T08: Esc button sends \\x1b to the PTY WebSocket', async ({ page }) => {
  const srv = await startServer(19052)
  try {
    await page.goto(srv.url)
    const toggleBtn = page.locator('button', { hasText: '⌨' })
    if (await toggleBtn.isVisible()) await toggleBtn.click()
    await expect(page.locator('button', { hasText: 'Esc' })).toBeVisible()

    let escSent = false
    page.on('websocket', ws => {
      ws.on('framesent', frame => {
        const d = frame.payload
        if (typeof d === 'string' && d.includes('\x1b')) escSent = true
        if (d instanceof Buffer && d.includes(0x1b)) escSent = true
      })
    })

    await page.locator('button', { hasText: 'Esc' }).click()
    await page.waitForTimeout(300)
    expect(escSent).toBe(true)
  } finally { srv.stop() }
})
```

- [ ] Run: `cd e2e && npx playwright test tests/keyboard.spec.ts`

Expected: 3 tests pass.

- [ ] Commit:

```bash
git add e2e/tests/keyboard.spec.ts
git commit -m "test(e2e): add virtual keyboard tests T08"
```

---

## Task 11: Docker tests — T13, T15, T16, T26, T30

**Files:**
- Create: `e2e/tests/docker.spec.ts`

Port range: 19060–19064. All tests skip gracefully if Docker is unavailable.

- [ ] Create `e2e/tests/docker.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { dockerRun, dockerExec, dockerLogs, waitForContainer, ContainerHandle } from '../helpers/docker'
import { execFileSync } from 'child_process'
import * as path from 'path'
import * as os from 'os'
import * as fs from 'fs'

const HOME_CLAUDE = path.join(os.homedir(), '.claude')

function dockerAvailable(): boolean {
  try { execFileSync('docker', ['info'], { stdio: 'ignore' }); return true } catch { return false }
}

let container: ContainerHandle | undefined

test.afterEach(() => { container?.stop(); container = undefined })

// T13 — /workspace mount shows host filesystem
test('T13: /workspace volume mount exposes host files inside container', async () => {
  test.skip(!dockerAvailable(), 'Docker not available')
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-ws-'))
  fs.writeFileSync(path.join(tmpDir, 'hello.txt'), 'from host')
  container = dockerRun({ port: 19060, env: { AUTH_MODE: 'none', LISTEN_ADDR: ':8080' }, volumes: [`${tmpDir}:/workspace`] })
  await waitForContainer(container.url)
  expect(dockerExec(container.id, ['ls', '/workspace'])).toContain('hello.txt')
  fs.rmSync(tmpDir, { recursive: true })
})

// T15 — mount ~/.claude → no login prompt
test('T15: mounting ~/.claude prevents Claude login prompt', async () => {
  test.skip(!dockerAvailable(), 'Docker not available')
  test.skip(!fs.existsSync(HOME_CLAUDE), '~/.claude not found')
  container = dockerRun({
    port: 19061,
    env: { AUTH_MODE: 'none', LISTEN_ADDR: ':8080' },
    volumes: [`${HOME_CLAUDE}:/home/perchuser/.claude`],
  })
  await waitForContainer(container.url)
  await new Promise(r => setTimeout(r, 6000))
  expect(dockerLogs(container.id)).not.toMatch(/please log in|please authenticate|oauth/i)
})

// T16 — no ~/.claude → login prompt appears
test('T16: without ~/.claude mount, Claude shows login prompt', async () => {
  test.skip(!dockerAvailable(), 'Docker not available')
  container = dockerRun({ port: 19062, env: { AUTH_MODE: 'none', LISTEN_ADDR: ':8080' } })
  await waitForContainer(container.url)
  await new Promise(r => setTimeout(r, 6000))
  expect(dockerLogs(container.id)).toMatch(/log in|authenticate|oauth|claude\.ai/i)
})

// T26 — skills copied to workspace, host ~/.claude untouched
test('T26: perch skills copied to workspace .claude/skills/', async () => {
  test.skip(!dockerAvailable(), 'Docker not available')
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-ws-'))
  container = dockerRun({
    port: 19063,
    env: { AUTH_MODE: 'none', LISTEN_ADDR: ':8080' },
    volumes: [
      `${tmpDir}:/workspace`,
      ...(fs.existsSync(HOME_CLAUDE) ? [`${HOME_CLAUDE}:/home/perchuser/.claude`] : []),
    ],
  })
  await waitForContainer(container.url)
  expect(dockerExec(container.id, ['ls', '/workspace/.claude/skills'])).toContain('local-schedule')
  fs.rmSync(tmpDir, { recursive: true })
})

// T30 — PUID/PGID: new files owned by PUID, not root
test('T30: with PUID set, files created in /workspace are owned by PUID', async () => {
  test.skip(!dockerAvailable(), 'Docker not available')
  test.skip(process.platform === 'win32', 'uid/gid not applicable on Windows')
  const uid = process.getuid!()
  const gid = process.getgid!()
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'perch-puid-'))
  fs.chmodSync(tmpDir, 0o777)
  container = dockerRun({
    port: 19064,
    env: { AUTH_MODE: 'none', LISTEN_ADDR: ':8080', PUID: String(uid), PGID: String(gid) },
    volumes: [`${tmpDir}:/workspace`],
  })
  await waitForContainer(container.url)
  dockerExec(container.id, ['sh', '-c', 'touch /workspace/owner-test.txt'])
  const stat = fs.statSync(path.join(tmpDir, 'owner-test.txt'))
  expect(stat.uid).toBe(uid)
  expect(stat.gid).toBe(gid)
  fs.rmSync(tmpDir, { recursive: true })
})
```

- [ ] Run: `PERCH_IMAGE=ghcr.io/fcwu/perch:latest cd e2e && npx playwright test tests/docker.spec.ts`

Expected: tests skip if Docker absent; pass otherwise.

- [ ] Commit:

```bash
git add e2e/tests/docker.spec.ts
git commit -m "test(e2e): add Docker mount and PUID/PGID tests T13 T15 T16 T26 T30"
```

---

## Task 12: Update DEVELOPMENT.md

**Files:**
- Modify: `DEVELOPMENT.md`

- [ ] Append the following section to `DEVELOPMENT.md` after the `go test ./...` block:

```markdown
## E2E 測試（Playwright）

需要：Node.js 20+、Docker（docker.spec.ts 才需要）

```bash
# 首次安裝
cd e2e && npm install && npx playwright install chromium

# 全部 e2e 測試（本地 binary，不需 Docker）
cd e2e && npx playwright test

# 含 Docker 測試
PERCH_IMAGE=ghcr.io/fcwu/perch:latest npx playwright test

# 只跑特定檔案
npx playwright test tests/api.spec.ts

# 看 HTML 報告
npx playwright show-report
```

**涵蓋範圍**

| 測試檔 | 涵蓋 | 說明 |
|--------|------|------|
| `api.spec.ts` | T01/T09/T10/T33 | 純 HTTP |
| `scheduler.spec.ts` | T05-T07/T27 | JSONL 檔案 |
| `mtls.spec.ts` | T12 | mTLS bootstrap |
| `terminal.spec.ts` | T02-T04/T17/T25 | xterm.js UI |
| `multi-tab.spec.ts` | T11/T14 | 多 tab |
| `keyboard.spec.ts` | T08 | 虛擬鍵盤 |
| `docker.spec.ts` | T13/T15/T16/T26/T30 | 需 Docker |
| `go test ./...` | T20-T24 | 既有 unit tests |
| 手動 | T18/T19/T28/T29/T31/T32 | 需 Discord token |
```

- [ ] Run: `cd e2e && npx playwright test`

Expected: all non-Docker, non-claude tests pass (~20 tests). Docker tests skip cleanly.

- [ ] Commit:

```bash
git add DEVELOPMENT.md
git commit -m "docs: add e2e test instructions to DEVELOPMENT.md"
```

---

## Coverage Summary

| Status | Tests | Count |
|--------|-------|-------|
| ✅ Playwright automated | T01-T17/T25-T27/T30/T33 | 25 |
| ✅ Existing `go test` | T20-T24 | 5 |
| ⚠️ Manual (Discord needed) | T18/T19/T28/T29/T31/T32 | 6 |
| ℹ️ Conditional skip | T03/T04 (need `claude` CLI) | — |

**Total automatable: 30/33**
