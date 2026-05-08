## Requirements

### Requirement: GITLAB_ALLOWED_IDS controls non-admin access in multi-user mode
In multi-user mode, `GITLAB_ALLOWED_IDS` determines which non-admin GitLab users may enter. Users in `GITLAB_ADMIN_IDS` are always allowed regardless of this setting.

| `GITLAB_ALLOWED_IDS` value | Behaviour |
|---|---|
| Not set or empty | Non-admin users are denied (deny all) |
| `*` | Any authenticated non-admin GitLab user is allowed |
| Comma-separated IDs | Only those specific IDs are allowed |

#### Scenario: GITLAB_ALLOWED_IDS not set — non-admin user is denied
- **WHEN** `PERCH_MODE=multi`, `GITLAB_ALLOWED_IDS` is unset, and the authenticated user is not in `GITLAB_ADMIN_IDS`
- **THEN** the server returns HTTP 403 and redirects to `/?error=access_denied`

#### Scenario: GITLAB_ALLOWED_IDS=* allows any authenticated non-admin user
- **WHEN** `PERCH_MODE=multi`, `GITLAB_ALLOWED_IDS=*`, and the authenticated user is not in `GITLAB_ADMIN_IDS`
- **THEN** the user is allowed and redirected to `/chat`

#### Scenario: Specific IDs — listed user is allowed
- **WHEN** `PERCH_MODE=multi`, `GITLAB_ALLOWED_IDS` is a comma-separated list, and the authenticated user's ID is in the list
- **THEN** the user is allowed and redirected to `/chat`

#### Scenario: Specific IDs — unlisted user is denied
- **WHEN** `PERCH_MODE=multi`, `GITLAB_ALLOWED_IDS` is a comma-separated list, and the authenticated user's ID is NOT in the list (and not in `GITLAB_ADMIN_IDS`)
- **THEN** the server returns HTTP 403 and redirects to `/?error=access_denied`

#### Scenario: Admin is always allowed regardless of GITLAB_ALLOWED_IDS
- **WHEN** `PERCH_MODE=multi` and the authenticated user's ID is in `GITLAB_ADMIN_IDS`
- **THEN** the user is allowed and redirected to `/admin`, regardless of `GITLAB_ALLOWED_IDS` value

### Requirement: GitLab auth middleware returns HTTP 401 JSON for unauthenticated API requests
The GitLab auth middleware SHALL return HTTP 401 with `{"error":"unauthorized"}` (not HTTP 302) when the caller has no valid session.

#### Scenario: Unauthenticated call to /api/chat returns 401
- **WHEN** a request to `POST /api/chat` has no valid session cookie
- **THEN** the server returns HTTP 401 with JSON error body, not a redirect

### Requirement: GET /auth/logout clears the session
The system SHALL provide a public `GET /auth/logout` endpoint that expires the session cookie and redirects to `/`.

#### Scenario: Logout clears cookie and redirects
- **WHEN** any user requests `GET /auth/logout`
- **THEN** the session cookie is cleared (MaxAge=-1) and the response redirects to `/`

### Requirement: Frontend shows inline login screen when unauthenticated in multi-user mode
When `GET /api/auth/status` returns `{"authenticated": false}` and `mode` is `multi`, the SPA SHALL render a centered login screen with a "Login with GitLab" button. No server-side redirect SHALL occur.

#### Scenario: Unauthenticated multi-user page load shows login screen
- **WHEN** the SPA loads in multi-user mode and auth status is false
- **THEN** a centered "Login with GitLab" screen is displayed

#### Scenario: access_denied error is shown on the login screen
- **WHEN** the URL contains `?error=access_denied`
- **THEN** the login screen displays "Access denied. Contact the administrator."

### Requirement: Frontend shows a logout button when authenticated
When `authenticated` is true, the SPA SHALL display a logout button. Clicking it navigates to `GET /auth/logout`.

#### Scenario: Logout button visible during authenticated session
- **WHEN** `GET /api/auth/status` returns `authenticated: true`
- **THEN** a logout button is visible in the UI

### Requirement: Frontend renders mode-appropriate UI based on auth status and role
The SPA SHALL use `GET /api/auth/status` on mount to determine which UI to render.

#### Scenario: Single-user mode always shows terminal UI after auth
- **WHEN** `mode` is `single` and `authenticated` is true (or AUTH_METHOD=none)
- **THEN** the SPA renders the terminal (Claude Code) UI at `/`

#### Scenario: Multi-user admin sees admin UI
- **WHEN** `mode` is `multi`, `authenticated` is true, and `role` is `admin`
- **THEN** the SPA at `/admin` renders the terminal UI and management panel

#### Scenario: Multi-user regular user sees chat UI
- **WHEN** `mode` is `multi`, `authenticated` is true, and `role` is `user`
- **THEN** the SPA at `/chat` renders the chat UI
