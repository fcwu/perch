## Requirements

### Requirement: AUTH_METHOD selects the authentication method for single-user mode
In single-user mode, the system SHALL read `AUTH_METHOD` at startup. Accepted values: `none` (default), `password`, `gitlab`. If `mtls` is provided, the server SHALL refuse to start with a clear error message.

#### Scenario: AUTH_METHOD=none allows unauthenticated access
- **WHEN** `AUTH_METHOD` is unset or `none`
- **THEN** all requests to `/` are served without authentication checks

#### Scenario: AUTH_METHOD=password requires HTTP Basic credentials
- **WHEN** `AUTH_METHOD=password` and a request to a protected route lacks valid Basic Auth credentials
- **THEN** the server returns HTTP 401 with `WWW-Authenticate: Basic realm="Perch"`

#### Scenario: Correct password issues a session cookie
- **WHEN** `AUTH_METHOD=password` and the request provides correct `PERCH_USERNAME` / `PERCH_PASSWORD` credentials
- **THEN** the server sets a session cookie and the subsequent request proceeds without re-prompting

#### Scenario: AUTH_METHOD=gitlab uses GitLab OAuth for single-user login
- **WHEN** `AUTH_METHOD=gitlab` and an unauthenticated user visits `/`
- **THEN** the SPA renders an inline login screen with a "Login with GitLab" button

#### Scenario: AUTH_METHOD=mtls is rejected at startup
- **WHEN** `AUTH_METHOD=mtls` is set
- **THEN** the server refuses to start and logs a clear error indicating mtls is no longer supported

### Requirement: PERCH_USERNAME defaults to "admin" for password auth
When `AUTH_METHOD=password`, `PERCH_USERNAME` env var sets the username (default: `admin`). `PERCH_PASSWORD` is required; the server refuses to start if it is unset.

#### Scenario: Missing PERCH_PASSWORD causes startup failure
- **WHEN** `AUTH_METHOD=password` and `PERCH_PASSWORD` is empty or unset
- **THEN** the server refuses to start and logs a configuration error

### Requirement: GITLAB_ADMIN_IDS restricts allowed accounts in single-user GitLab auth
When `AUTH_METHOD=gitlab` in single-user mode, `GITLAB_ADMIN_IDS` (comma-separated GitLab user IDs) restricts which accounts may complete OAuth. If unset, any authenticated GitLab user is allowed.

#### Scenario: Non-listed user is rejected in single-user GitLab auth
- **WHEN** `AUTH_METHOD=gitlab`, `GITLAB_ADMIN_IDS` is set, and the authenticated user's GitLab ID is absent
- **THEN** the server returns HTTP 403 and does NOT set a session cookie
