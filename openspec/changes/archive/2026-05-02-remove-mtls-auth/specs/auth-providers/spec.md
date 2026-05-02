## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: AUTH_METHOD=mtls bootstrap flow
**Reason**: mTLS 認證已移除。外層安全方案（Cloudflare Zero Trust、VPN）可提供同等保護，且無需管理 client certificate。
**Migration**: 將 `AUTH_METHOD=mtls` 改為 `none`（內網部署）或 `password`（密碼保護）。`/bootstrap` 路由不再存在。
