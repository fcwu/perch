#!/usr/bin/env python3
"""
GitLab code fetcher - supports multiple GitLab instances.
Fetches MR diffs or commit diffs from configured GitLab instances.
"""

import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Optional
from urllib.parse import quote, urlparse

# Config file location (user can customize)
CONFIG_PATH = Path.home() / ".config" / "git-review" / "gitlab.json"


def load_config() -> dict:
    """Load GitLab configuration from file."""
    if not CONFIG_PATH.exists():
        # Return empty config, will rely on environment variables
        return {"instances": {}, "default_instance": None}

    with open(CONFIG_PATH) as f:
        return json.load(f)


def get_instance_config(url: str, config: dict) -> tuple[str, str, bool]:
    """
    Get GitLab instance URL, token, and SSL verify setting for a given URL.
    Returns (base_url, token, skip_ssl_verify).
    """
    parsed = urlparse(url)
    host = parsed.netloc

    # Check config file first
    instances = config.get("instances", {})
    if host in instances:
        inst = instances[host]
        return inst["url"], inst["token"], inst.get("skip_ssl_verify", False)

    # Fallback to environment variables
    # Try host-specific env vars first: GITLAB_TOKEN_GITLAB_EXAMPLE_COM
    env_key = f"GITLAB_TOKEN_{host.upper().replace('.', '_').replace('-', '_')}"
    token = os.environ.get(env_key)

    # Check for skip SSL verify env var
    ssl_env_key = f"GITLAB_SKIP_SSL_{host.upper().replace('.', '_').replace('-', '_')}"
    skip_ssl = os.environ.get(ssl_env_key, "").lower() in ("true", "1", "yes")

    if not skip_ssl:
        skip_ssl = os.environ.get("GITLAB_SKIP_SSL_VERIFY", "").lower() in ("true", "1", "yes")

    if token:
        return f"https://{host}", token, skip_ssl

    # Try generic GITLAB_TOKEN
    token = os.environ.get("GITLAB_TOKEN")
    if token:
        return f"https://{host}", token, skip_ssl

    raise ValueError(
        f"No token found for GitLab instance: {host}. "
        f"Set {env_key} or GITLAB_TOKEN environment variable, "
        f"or add to config at {CONFIG_PATH}"
    )


def gitlab_api_request(base_url: str, endpoint: str, token: str, skip_ssl_verify: bool = False) -> dict:
    """Make a GitLab API request."""
    url = f"{base_url}/api/v4{endpoint}"
    headers = {"PRIVATE-TOKEN": token}

    req = urllib.request.Request(url, headers=headers)

    # Handle SSL
    if skip_ssl_verify:
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
    else:
        ctx = ssl.create_default_context()

    try:
        with urllib.request.urlopen(req, context=ctx, timeout=30) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        error_body = e.read().decode() if e.fp else ""
        raise RuntimeError(f"GitLab API error {e.code}: {error_body}") from e


def parse_gitlab_url(url: str) -> dict:
    """
    Parse GitLab URL to extract project and MR/commit info.

    Supports:
    - https://gitlab.com/group/project/-/merge_requests/123
    - https://gitlab.com/group/subgroup/project/-/merge_requests/123
    - https://gitlab.com/group/project/-/commit/abc123
    - https://gitlab.com/group/project/-/commits/abc123 (alternative format)
    """
    parsed = urlparse(url)
    path = parsed.path

    result = {
        "host": parsed.netloc,
        "type": None,
        "project_path": None,
        "mr_iid": None,
        "commit_sha": None,
    }

    # Match MR URL
    mr_match = re.match(r"^/(.+?)/-/merge_requests/(\d+)", path)
    if mr_match:
        result["type"] = "merge_request"
        result["project_path"] = mr_match.group(1)
        result["mr_iid"] = mr_match.group(2)
        return result

    # Match commit URL
    commit_match = re.match(r"^/(.+?)/-/commits?/([a-f0-9]+)", path)
    if commit_match:
        result["type"] = "commit"
        result["project_path"] = commit_match.group(1)
        result["commit_sha"] = commit_match.group(2)
        return result

    raise ValueError(f"Unable to parse GitLab URL: {url}")


def extract_jira_ticket(text: str, project_prefixes: list[str] = None) -> Optional[str]:
    """
    Extract JIRA ticket ID from text.
    Default pattern: [A-Z][A-Z0-9]+-\d+ (e.g., TEM61510-12345)

    Args:
        text: Text to search (MR title, description, commit message)
        project_prefixes: Optional list of known project prefixes to prioritize
    """
    if project_prefixes:
        # Try specific prefixes first
        for prefix in project_prefixes:
            pattern = rf"\b({re.escape(prefix)}-\d+)\b"
            match = re.search(pattern, text, re.IGNORECASE)
            if match:
                return match.group(1).upper()

    # Generic JIRA pattern
    pattern = r"\b([A-Z][A-Z0-9]+-\d+)\b"
    match = re.search(pattern, text)
    return match.group(1) if match else None


def fetch_mr_info(base_url: str, token: str, project_path: str, mr_iid: str, skip_ssl_verify: bool = False) -> dict:
    """Fetch merge request information."""
    encoded_project = quote(project_path, safe="")

    # Get MR details
    mr_data = gitlab_api_request(
        base_url,
        f"/projects/{encoded_project}/merge_requests/{mr_iid}",
        token,
        skip_ssl_verify,
    )

    # Get MR diffs
    diffs = gitlab_api_request(
        base_url,
        f"/projects/{encoded_project}/merge_requests/{mr_iid}/diffs",
        token,
        skip_ssl_verify,
    )

    return {
        "type": "merge_request",
        "title": mr_data.get("title", ""),
        "description": mr_data.get("description", ""),
        "source_branch": mr_data.get("source_branch", ""),
        "target_branch": mr_data.get("target_branch", ""),
        "author": mr_data.get("author", {}).get("username", ""),
        "web_url": mr_data.get("web_url", ""),
        "diffs": diffs.get("diffs", []),
    }


def fetch_commit_info(base_url: str, token: str, project_path: str, commit_sha: str, skip_ssl_verify: bool = False) -> dict:
    """Fetch commit information."""
    encoded_project = quote(project_path, safe="")

    # Get commit details
    commit_data = gitlab_api_request(
        base_url,
        f"/projects/{encoded_project}/repository/commits/{commit_sha}",
        token,
        skip_ssl_verify,
    )

    # Get commit diff
    diffs = gitlab_api_request(
        base_url,
        f"/projects/{encoded_project}/repository/commits/{commit_sha}/diff",
        token,
        skip_ssl_verify,
    )

    return {
        "type": "commit",
        "title": commit_data.get("title", ""),
        "message": commit_data.get("message", ""),
        "author": commit_data.get("author_name", ""),
        "web_url": commit_data.get("web_url", ""),
        "diffs": diffs,
    }


def format_diffs(diffs: list[dict]) -> str:
    """Format diffs for output."""
    output = []
    for diff in diffs:
        path = diff.get("new_path") or diff.get("old_path", "unknown")
        output.append(f"\n{'=' * 60}")
        output.append(f"File: {path}")

        if diff.get("new_file"):
            output.append("(new file)")
        elif diff.get("deleted_file"):
            output.append("(deleted)")
        elif diff.get("renamed_file"):
            old_path = diff.get("old_path", "")
            output.append(f"(renamed from {old_path})")

        output.append("-" * 60)
        output.append(diff.get("diff", ""))

    return "\n".join(output)


def fetch_code_changes(url: str) -> dict:
    """
    Main entry point: fetch code changes from GitLab URL.

    Returns dict with:
    - type: "merge_request" or "commit"
    - title: MR/commit title
    - description/message: Full description/message
    - jira_ticket: Extracted JIRA ticket (if found)
    - diffs_formatted: Formatted diff string
    - raw_diffs: Raw diff data
    """
    config = load_config()
    parsed = parse_gitlab_url(url)
    base_url, token, skip_ssl = get_instance_config(url, config)

    if parsed["type"] == "merge_request":
        data = fetch_mr_info(base_url, token, parsed["project_path"], parsed["mr_iid"], skip_ssl)
    else:
        data = fetch_commit_info(base_url, token, parsed["project_path"], parsed["commit_sha"], skip_ssl)

    # Extract JIRA ticket
    text_to_search = f"{data.get('title', '')} {data.get('description', '')} {data.get('message', '')}"
    jira_ticket = extract_jira_ticket(text_to_search)

    return {
        "type": data["type"],
        "title": data.get("title", ""),
        "description": data.get("description", ""),
        "message": data.get("message", ""),
        "author": data.get("author", ""),
        "web_url": data.get("web_url", ""),
        "jira_ticket": jira_ticket,
        "diffs_formatted": format_diffs(data["diffs"]),
        "raw_diffs": data["diffs"],
    }


def main():
    """CLI interface."""
    if len(sys.argv) < 2:
        print("Usage: gitlab_fetcher.py <gitlab_url>")
        print("\nExamples:")
        print("  gitlab_fetcher.py https://gitlab.com/group/project/-/merge_requests/123")
        print("  gitlab_fetcher.py https://gitlab.com/group/project/-/commit/abc123")
        print("\nEnvironment variables:")
        print("  GITLAB_TOKEN              - Default GitLab token")
        print("  GITLAB_TOKEN_<HOST>       - Token for specific host (e.g., GITLAB_TOKEN_GITLAB_EXAMPLE_COM)")
        print("  GITLAB_SKIP_SSL_VERIFY    - Skip SSL verification for all hosts (true/false)")
        print("  GITLAB_SKIP_SSL_<HOST>    - Skip SSL verification for specific host")
        sys.exit(1)

    url = sys.argv[1]

    try:
        result = fetch_code_changes(url)

        print(f"Type: {result['type']}")
        print(f"Title: {result['title']}")
        print(f"Author: {result['author']}")
        print(f"URL: {result['web_url']}")

        if result["jira_ticket"]:
            print(f"JIRA Ticket: {result['jira_ticket']}")
        else:
            print("JIRA Ticket: Not found in title/description")

        if result.get("description"):
            print(f"\nDescription:\n{result['description']}")

        if result.get("message"):
            print(f"\nCommit Message:\n{result['message']}")

        print(f"\n{'=' * 60}")
        print("CODE CHANGES:")
        print(result["diffs_formatted"])

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
