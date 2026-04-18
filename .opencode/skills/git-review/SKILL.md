---
name: git-review
description: Compare GitLab MR or commit implementation against JIRA spec for functional completeness review. Use when reviewing merge requests or commits to verify implementation matches requirements. Triggers on MR/commit review requests with JIRA ticket references. Outputs checklist-style comments.
---

# Spec Review Skill

Compare GitLab merge request or commit implementation against JIRA ticket specifications to verify functional completeness.

## Workflow

1. **Receive GitLab URL** (MR or commit) from user
2. **If `.venv` exists** in the working directory, activate it
3. **Extract JIRA ticket ID** from MR title/description or commit message
4. **Fetch spec** from JIRA (description, attachments, Confluence links in description)
5. **Fetch code changes** from GitLab
6. **Compare** implementation against spec requirements
7. **Generate checklist comment** for functional completeness, including completion percentage

## Setup

### Required Python Packages

```bash
# 使用 uv (推薦，更快)
if [ -d .venv ]; then
  . .venv/bin/activate
fi
uv pip install -r requirements.txt

# 或用 pip
if [ -d .venv ]; then
  . .venv/bin/activate
fi
pip install -r requirements.txt

# 或手動安裝
if [ -d .venv ]; then
  . .venv/bin/activate
fi
uv pip install jira atlassian-python-api beautifulsoup4
```

### Environment Variables (Quick Start)

```bash
# GitLab
export GITLAB_TOKEN="glpat-xxx"
export GITLAB_SKIP_SSL_VERIFY="true"  # For internal servers

# JIRA (token authentication only)
export JIRA_URL="https://jira.mycompany.com"
export JIRA_TOKEN="your-jira-personal-access-token"
export JIRA_SKIP_SSL_VERIFY="true"  # For internal servers

# Confluence (separate token)
export CONFLUENCE_URL="https://confluence.mycompany.com"
export CONFLUENCE_TOKEN="your-confluence-personal-access-token"
export CONFLUENCE_SKIP_SSL_VERIFY="true"  # For internal servers
```

### Config Files (Multiple Instances)

Copy example configs to `~/.config/git-review/`:

```bash
mkdir -p ~/.config/git-review
cp references/gitlab.example.json ~/.config/git-review/gitlab.json
cp references/jira.example.json ~/.config/git-review/jira.json
# Edit with your credentials
```

**GitLab config** (`gitlab.json`): Supports multiple instances with `skip_ssl_verify` per instance.

**JIRA/Confluence config** (`jira.json`): Separate `jira` and `confluence` sections, each with own `url`, `token`, and `skip_ssl_verify`.

## Usage

When user provides a GitLab URL:

```
1. If `.venv` exists, run: . .venv/bin/activate
2. Run: python3 scripts/gitlab_fetcher.py <gitlab_url>
3. Extract JIRA ticket ID from output (or ask user if not found)
4. Run: python3 scripts/jira_fetcher.py <ticket_id>
5. Compare spec requirements against code changes
6. Generate checklist comment
```

## Spec Sources

The JIRA fetcher automatically collects spec from:
- **JIRA Description** - Main ticket description
- **Attachments** - Text-based files (.txt, .md, .json, etc.)
- **Confluence Links** - URLs found in description are auto-fetched
- **Remote Links** - Linked Confluence pages

## JIRA Ticket ID Patterns

Common patterns: `TEM61510-12345`, `PROJ-123`, `ABC123-456`

Pattern: `[A-Z][A-Z0-9]+-\d+`

If ticket ID not found in MR/commit, ask user to provide it.

## Comment Output Format

Generate checklist-style comments focusing on **functional completeness**. Include a completion percentage computed as (implemented + 0.5 * partial) / total requirements, rounded to a whole percent.

```markdown
## Spec Review: {TICKET_ID}

**Completion:** 67% (partial counts as 0.5 of a requirement)

### ✅ Implemented Requirements
- [x] Requirement 1 - implemented in `file.go:123`
- [x] Requirement 2 - covered by changes in `handler.go`

### ⚠️ Partially Implemented
- [ ] Requirement 3 - missing error handling for edge case X
- [ ] Requirement 4 - API endpoint created but validation incomplete

### ❌ Not Implemented
- [ ] Requirement 5 - no changes related to this requirement
- [ ] Requirement 6 - mentioned in spec but not addressed

### 📝 Notes
- Additional observations about implementation quality
- Suggestions for improvement
```

## Review Focus Areas

When comparing spec vs implementation:

1. **API Changes**: Endpoints, parameters, response format match spec
2. **Business Logic**: Core functionality requirements fulfilled
3. **Error Handling**: Error cases mentioned in spec are handled
4. **Data Validation**: Input validation per spec requirements
5. **Edge Cases**: Boundary conditions addressed
6. **Missing Features**: Spec items with no corresponding code changes
