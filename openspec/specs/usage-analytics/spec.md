## Requirements

### Requirement: Usage statistics API

`GET /admin/analytics` SHALL return aggregated usage statistics for a given time range.

#### Scenario: per-user stats requested
- **WHEN** admin calls `GET /admin/analytics?from=<unix_ms>&to=<unix_ms>`
- **THEN** the server SHALL return:
  ```json
  {
    "users": [
      {"username":"alice","query_count":42,"avg_duration_ms":3100},
      ...
    ],
    "top_tools": [
      {"tool":"read","count":210},
      {"tool":"bash","count":5}
    ],
    "total_queries": 150,
    "total_duration_ms": 465000
  }
  ```
  sorted by `query_count` descending for users, and by `count` descending for tools (top 10)

#### Scenario: no data in range
- **WHEN** no sessions exist within the specified time range
- **THEN** the server SHALL return `{"users":[],"top_tools":[],"total_queries":0,"total_duration_ms":0}`

### Requirement: Analytics UI

The admin panel SHALL include an Analytics tab at `/admin/analytics`.

#### Scenario: time range selector
- **WHEN** admin navigates to the Analytics tab
- **THEN** the UI SHALL show preset ranges (Today, This Week, This Month) and call the API accordingly

#### Scenario: results displayed
- **WHEN** the API returns data
- **THEN** the UI SHALL display a user stats table (username, query count, avg duration) and a top tools table, sorted as returned by the API
