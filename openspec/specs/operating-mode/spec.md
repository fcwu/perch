## Requirements

### Requirement: PERCH_MODE selects single or multi-user operating mode
The system SHALL read the `PERCH_MODE` environment variable at startup. Accepted values are `single` (default) and `multi`. Any other value SHALL cause the server to refuse to start with a descriptive error.

#### Scenario: Default mode is single-user
- **WHEN** `PERCH_MODE` is unset or empty
- **THEN** the server starts in single-user mode

#### Scenario: Explicit multi-user mode
- **WHEN** `PERCH_MODE=multi`
- **THEN** the server starts in multi-user mode

#### Scenario: Multi-user mode requires GitLab OAuth configuration
- **WHEN** `PERCH_MODE=multi` and any of `GITLAB_CLIENT_ID`, `GITLAB_CLIENT_SECRET`, or `GITLAB_URL` is missing
- **THEN** the server refuses to start and logs a clear configuration error

### Requirement: Single-user mode serves the terminal UI at /
In single-user mode, the server SHALL serve the terminal (Claude Code) UI at `/` after the configured authentication check passes.

#### Scenario: Authenticated user reaches terminal UI
- **WHEN** the server is in single-user mode and the request is authenticated (or AUTH_METHOD=none)
- **THEN** `GET /` returns `index.html` and the SPA renders the terminal UI

### Requirement: Multi-user mode routes users by role after login
In multi-user mode, after successful GitLab OAuth the server SHALL redirect the user based on their role: admins (in `GITLAB_ADMIN_IDS`) to `/admin`, all others to `/chat`.

#### Scenario: Admin user is redirected to /admin after OAuth
- **WHEN** the server is in multi-user mode and the authenticated user's GitLab ID is in `GITLAB_ADMIN_IDS`
- **THEN** the OAuth callback redirects to `/admin`

#### Scenario: Regular user is redirected to /chat after OAuth
- **WHEN** the server is in multi-user mode and the authenticated user's GitLab ID is NOT in `GITLAB_ADMIN_IDS`
- **THEN** the OAuth callback redirects to `/chat`

#### Scenario: Admin can access /chat directly
- **WHEN** the server is in multi-user mode and an admin navigates directly to `/chat`
- **THEN** the page loads and the API accepts the admin's session

### Requirement: HTML shell routes are served without auth enforcement
The server SHALL serve `index.html` for `GET /`, `GET /chat`, and `GET /admin` regardless of authentication state. Auth enforcement is on API routes only.

#### Scenario: Unauthenticated browser loads /chat in multi-user mode
- **WHEN** an unauthenticated user requests `GET /chat` in multi-user mode
- **THEN** the server returns HTTP 200 with `index.html`

#### Scenario: GET /api/auth/status is public and always returns 200
- **WHEN** any client requests `GET /api/auth/status`
- **THEN** the server returns HTTP 200 with a JSON body containing `authenticated`, `username`, `role`, and `mode` fields
