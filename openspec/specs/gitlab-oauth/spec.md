## Requirements

### Requirement: GitLab OAuth2 login flow

The server SHALL implement GitLab OAuth2 Authorization Code Flow with the following endpoints:
- `GET /auth/gitlab` — redirect user to GitLab authorization URL
- `GET /auth/callback` — receive code, exchange for token, set signed session cookie, redirect to `/chat`

Configuration SHALL be read from env vars: `GITLAB_URL`, `GITLAB_CLIENT_ID`, `GITLAB_CLIENT_SECRET`, `GITLAB_REDIRECT_URI`.

#### Scenario: successful login
- **WHEN** a user visits `/auth/gitlab` and completes GitLab OAuth consent
- **THEN** the server SHALL exchange the code for a GitLab token, fetch the user's GitLab profile (id, username), set a signed session cookie (`perch_session`) with `userID`, `username`, and `exp` (8h TTL), and redirect to `/chat`

#### Scenario: invalid callback state
- **WHEN** the OAuth callback contains a mismatched or missing `state` parameter
- **THEN** the server SHALL return HTTP 400 and NOT set a session cookie

#### Scenario: GitLab token exchange failure
- **WHEN** GitLab returns an error during code exchange
- **THEN** the server SHALL return HTTP 502 with an error message

### Requirement: Session cookie authentication

`/chat` and `/ws/chat` endpoints SHALL require a valid signed session cookie. All other existing routes (`/`, `/ws`, `/hook`, `/sessions`, `/ws/session`) SHALL continue to use the existing `AUTH_MODE`-based middleware and SHALL NOT be affected by this change.

#### Scenario: valid session cookie
- **WHEN** a request to `/chat` or `/ws/chat` includes a valid, non-expired `perch_session` cookie
- **THEN** the server SHALL allow the request and populate the request context with `userID` and `username`

#### Scenario: missing or expired cookie
- **WHEN** a request to `/chat` or `/ws/chat` has no cookie or an expired cookie
- **THEN** the server SHALL redirect to `/auth/gitlab`

#### Scenario: tampered cookie
- **WHEN** the HMAC signature on `perch_session` does not match
- **THEN** the server SHALL return HTTP 401
