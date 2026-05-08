## Requirements

### Requirement: Admin token login

The server SHALL provide `POST /admin/login` accepting `{"token": "<value>"}` and validating against the `ADMIN_TOKEN` environment variable.

#### Scenario: correct token
- **WHEN** the posted token matches `ADMIN_TOKEN`
- **THEN** the server SHALL set a signed `perch_admin` cookie (TTL 24h) and return HTTP 200

#### Scenario: incorrect token
- **WHEN** the posted token does not match `ADMIN_TOKEN`
- **THEN** the server SHALL return HTTP 401 with no cookie set

### Requirement: Admin session middleware

All `/admin/*` routes (except `/admin/login`) SHALL require a valid `perch_admin` cookie.

#### Scenario: valid admin cookie
- **WHEN** a request carries a valid, non-expired `perch_admin` cookie
- **THEN** the request SHALL proceed to the admin handler

#### Scenario: missing or expired admin cookie
- **WHEN** a request to `/admin/*` (non-login) has no valid cookie
- **THEN** the server SHALL return HTTP 401; the admin frontend SHALL redirect to the login page
