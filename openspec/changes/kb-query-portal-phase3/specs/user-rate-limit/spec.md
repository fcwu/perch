## ADDED Requirements

### Requirement: Per-user query rate limiting

The server SHALL enforce a per-user rate limit on `POST /api/chat`, configurable via `RATE_LIMIT_RPM` (requests per minute, default 10).

#### Scenario: user within limit
- **WHEN** an authenticated user submits a query within their allowed rate
- **THEN** the request SHALL proceed normally

#### Scenario: user exceeds limit
- **WHEN** an authenticated user submits a query that exceeds `RATE_LIMIT_RPM`
- **THEN** the server SHALL return HTTP 429 with body `{"error":"rate limit exceeded","retry_after_ms":<N>}` where `retry_after_ms` is the time until the next token is available

#### Scenario: RATE_LIMIT_RPM=0 disables limiting
- **WHEN** `RATE_LIMIT_RPM` is set to `0`
- **THEN** no rate limiting SHALL be applied

### Requirement: Rate limiter isolation per user

Each user's rate limit SHALL be tracked independently; one user's queries SHALL NOT consume another user's quota.

#### Scenario: two users query simultaneously
- **WHEN** user A and user B both send queries at the same time
- **THEN** each SHALL be evaluated against their own limiter; neither SHALL be blocked by the other's usage
