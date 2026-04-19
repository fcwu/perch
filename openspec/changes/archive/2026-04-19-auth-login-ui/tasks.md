## 1. Core — Operating Mode

- [x] 1.1 Define `OperatingMode` type (`single`|`multi`) and read `PERCH_MODE` env var in `main.go` or new `mode.go`; default to `single`
- [x] 1.2 If `PERCH_MODE=multi` and any of `GITLAB_CLIENT_ID`/`GITLAB_CLIENT_SECRET`/`GITLAB_URL` is missing, log error and exit
- [x] 1.3 Pass `OperatingMode` into `NewServer` so routing decisions are mode-aware
- [x] 1.4 Unit-test mode parsing: invalid value returns error; missing GitLab vars in multi-mode returns error

## 2. Server — GET /api/auth/status

- [x] 2.1 Add `handleAuthStatus` in a new or existing auth file — reads session cookie, returns `{"authenticated":bool,"username":"","role":"admin|user|","mode":"single|multi"}` always HTTP 200
- [x] 2.2 Register `/api/auth/status` as a public route in `server.go`
- [x] 2.3 Unit-test: valid admin session → role=admin; valid user session → role=user; no session → authenticated=false

## 3. Server — GET /auth/logout

- [x] 3.1 Add `handleLogout` in `gitlab_auth.go` — set `perch_session` MaxAge=-1, redirect to `/`
- [x] 3.2 Register `GET /auth/logout` as a public route; add to exempt list in ServeHTTP
- [x] 3.3 Unit-test: response clears cookie and redirects to `/`

## 4. Server — GitLab Auth Updates

- [x] 4.1 Add `adminIDs map[string]bool` and `allowedIDs map[string]bool` to `gitLabAuth`; parse `GITLAB_ADMIN_IDS` and `GITLAB_ALLOWED_IDS` in `newGitLabAuth`
- [x] 4.2 In `handleCallback` (multi-user mode): if user is in adminIDs → allow; else check allowedIDs: empty/unset → 403, `"*"` → allow, comma list → check membership; reject non-matching with 403 + redirect to `/?error=access_denied`
- [x] 4.3 In `handleCallback`: after successful auth, set `role` claim in session cookie (`admin` if in adminIDs, else `user`); redirect admin → `/admin`, user → `/chat`
- [x] 4.4 In `handleCallback` (single-user GitLab mode): check adminIDs as allowlist; redirect to `/` on success
- [x] 4.5 Change `gitlabAuth.middleware` to return HTTP 401 + `{"error":"unauthorized"}` instead of HTTP 302
- [x] 4.6 Update `gitlab_auth_test.go`: middleware tests expecting 302 → update to 401

## 5. Server — Single-User Auth Providers

- [x] 5.1 Read `AUTH_METHOD` env var; validate value is one of `none|password|mtls|gitlab`; default to `none`
- [x] 5.2 Implement `AUTH_METHOD=password`: read `PERCH_USERNAME` (default `admin`) and `PERCH_PASSWORD`; refuse to start if password unset; check Basic Auth credentials; issue session cookie on success
- [x] 5.3 Implement `AUTH_METHOD=mtls`: verify `tls.go` is configured; require client cert in TLS config (`tls.RequireAnyClientCert`); refuse to start if TLS not enabled
- [x] 5.4 `AUTH_METHOD=none`: no middleware applied to `/`
- [x] 5.5 `AUTH_METHOD=gitlab`: use existing GitLab OAuth flow with single-user semantics (adminIDs as allowlist, redirect to `/` after auth)

## 6. Server — Mode-Aware Route Wiring

- [x] 6.1 In single-user mode: apply appropriate auth middleware to `/api/*` based on `AUTH_METHOD`; serve `index.html` for `GET /` without auth
- [x] 6.2 In multi-user mode: serve `index.html` for `GET /`, `GET /chat`, `GET /admin` without auth; API routes use `gitlabAuth.middleware`
- [x] 6.3 Remove old unconditional `gitlabAuth.middleware` from `/chat` HTML route
- [x] 6.4 Add `/api/auth/status`, `/auth/logout`, `/auth/gitlab`, `/auth/callback` to exempt-from-primary-auth list

## 7. Frontend — Auth Status Check

- [x] 7.1 On app mount, call `GET /api/auth/status` and store result in top-level state (`authenticated`, `username`, `role`, `mode`)
- [x] 7.2 Show loading spinner while status fetch is in-flight
- [x] 7.3 Parse `?error=access_denied` from URL on mount and store as error state

## 8. Frontend — Login Screen (multi-user unauthenticated)

- [x] 8.1 When `mode=multi` and `authenticated=false`, render centered inline login screen matching dark theme
- [x] 8.2 "Login with GitLab" button navigates to `/auth/gitlab`
- [x] 8.3 When `error=access_denied`, display "Access denied. Contact the administrator." on the login screen

## 9. Frontend — Mode-Aware UI Rendering

- [x] 9.1 Single-user / terminal UI: render existing terminal component at `/` when `authenticated=true` (or `AUTH_METHOD=none`)
- [x] 9.2 Multi-user admin (`role=admin` at `/admin`): render terminal UI + link/tab to management panel
- [x] 9.3 Multi-user user (`role=user` at `/chat`): render chat UI (existing ChatPage)
- [x] 9.4 Add logout button to all authenticated views; navigates to `/auth/logout`

## 10. Verification

- [x] 10.1 Manual test single/none: `/` opens terminal UI without login prompt
- [x] 10.2 Manual test single/password: wrong password → 401; correct password → terminal UI
- [x] 10.3 Manual test multi: unauthenticated → login screen; login as admin → `/admin` with terminal; login as regular user → `/chat`
- [x] 10.4 Manual test multi: GITLAB_ALLOWED_IDS set; unlisted user → access denied screen
- [x] 10.5 Manual test: logout button → login screen
- [x] 10.6 Run full test suite (`go test ./...`) — all existing tests pass
