#!/usr/bin/env python3
"""
JIRA/Confluence spec fetcher using official Python libraries with token authentication.
Fetches specification from JIRA ticket including description, attachments, and Confluence pages.

Required packages:
    pip install jira atlassian-python-api
"""

import json
import os
import re
import sys
import warnings
from pathlib import Path
from typing import Optional

import urllib3

# Config file location
CONFIG_PATH = Path.home() / ".config" / "git-review" / "jira.json"


def load_config() -> dict:
    """Load JIRA/Confluence configuration."""
    config = {"jira": {}, "confluence": {}}

    # Try config file first
    if CONFIG_PATH.exists():
        with open(CONFIG_PATH) as f:
            file_config = json.load(f)
            config["jira"] = file_config.get("jira", {})
            config["confluence"] = file_config.get("confluence", {})

    # Override with environment variables if set
    # JIRA
    if os.environ.get("JIRA_URL"):
        config["jira"]["url"] = os.environ["JIRA_URL"]
    if os.environ.get("JIRA_TOKEN"):
        config["jira"]["token"] = os.environ["JIRA_TOKEN"]
    if os.environ.get("JIRA_SKIP_SSL_VERIFY"):
        config["jira"]["skip_ssl_verify"] = os.environ["JIRA_SKIP_SSL_VERIFY"].lower() in ("true", "1", "yes")

    # Confluence
    if os.environ.get("CONFLUENCE_URL"):
        config["confluence"]["url"] = os.environ["CONFLUENCE_URL"]
    if os.environ.get("CONFLUENCE_TOKEN"):
        config["confluence"]["token"] = os.environ["CONFLUENCE_TOKEN"]
    if os.environ.get("CONFLUENCE_SKIP_SSL_VERIFY"):
        config["confluence"]["skip_ssl_verify"] = os.environ["CONFLUENCE_SKIP_SSL_VERIFY"].lower() in ("true", "1", "yes")

    return config


def get_jira_client(config: dict):
    """Create JIRA client instance using token authentication."""
    try:
        from jira import JIRA
    except ImportError:
        raise ImportError("Please install jira package: pip install jira")

    jira_config = config.get("jira", {})

    url = jira_config.get("url")
    if not url:
        raise ValueError("JIRA URL not configured. Set JIRA_URL environment variable or configure in config file.")

    token = jira_config.get("token")
    if not token:
        raise ValueError("JIRA token not configured. Set JIRA_TOKEN environment variable or configure in config file.")

    skip_ssl = jira_config.get("skip_ssl_verify", False)

    # Suppress SSL warnings if skipping verification
    if skip_ssl:
        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
        warnings.filterwarnings("ignore", message="Unverified HTTPS request")

    jira_options = {
        "server": url,
        "verify": not skip_ssl,
    }

    # Use token authentication
    return JIRA(options=jira_options, token_auth=token)


def get_confluence_client(config: dict):
    """Create Confluence client instance using token authentication."""
    try:
        from atlassian import Confluence
    except ImportError:
        raise ImportError("Please install atlassian-python-api: pip install atlassian-python-api")

    confluence_config = config.get("confluence", {})

    url = confluence_config.get("url")
    if not url:
        raise ValueError("Confluence URL not configured. Set CONFLUENCE_URL environment variable or configure in config file.")

    token = confluence_config.get("token")
    if not token:
        raise ValueError("Confluence token not configured. Set CONFLUENCE_TOKEN environment variable or configure in config file.")

    skip_ssl = confluence_config.get("skip_ssl_verify", False)

    if skip_ssl:
        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
        warnings.filterwarnings("ignore", message="Unverified HTTPS request")

    # Use token authentication (Personal Access Token)
    return Confluence(
        url=url,
        token=token,
        verify_ssl=not skip_ssl,
    )


def extract_confluence_links(text: str) -> list[str]:
    """Extract Confluence page URLs from text."""
    if not text:
        return []

    # Common Confluence URL patterns
    patterns = [
        r"https?://[^\s\)\]<>]+/wiki/spaces/[^\s\)\]<>]+/pages/\d+[^\s\)\]<>]*",
        r"https?://[^\s\)\]<>]+/wiki/display/[^\s\)\]<>]+/[^\s\)\]<>]+",
        r"https?://[^\s\)\]<>]+/pages/viewpage\.action\?pageId=\d+",
        r"https?://[^\s\)\]<>]+\.atlassian\.net/wiki/[^\s\)\]<>]+",
    ]

    links = []
    for pattern in patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        links.extend(matches)

    # Clean up links (remove trailing punctuation)
    cleaned = []
    for link in links:
        link = re.sub(r"[.,;:!?\'\"]+$", "", link)
        cleaned.append(link)

    return list(set(cleaned))


def extract_page_id_from_url(url: str) -> Optional[str]:
    """Extract page ID from Confluence URL."""
    # Pattern: /pages/123456 or pageId=123456
    match = re.search(r"/pages/(\d+)", url)
    if match:
        return match.group(1)

    match = re.search(r"pageId=(\d+)", url)
    if match:
        return match.group(1)

    return None


def fetch_confluence_page(confluence, url: str) -> Optional[dict]:
    """Fetch Confluence page content."""
    page_id = extract_page_id_from_url(url)

    if not page_id:
        # Try to get page by URL path for display URLs
        # e.g., /wiki/display/SPACE/Page+Title
        match = re.search(r"/display/([^/]+)/(.+?)(?:\?|$)", url)
        if match:
            space_key = match.group(1)
            title = match.group(2).replace("+", " ").replace("%20", " ")
            try:
                page = confluence.get_page_by_title(space_key, title, expand="body.storage")
                if page:
                    return {
                        "title": page.get("title", ""),
                        "url": url,
                        "content": page.get("body", {}).get("storage", {}).get("value", ""),
                        "content_text": html_to_text(page.get("body", {}).get("storage", {}).get("value", "")),
                    }
            except Exception as e:
                return {"title": "Error", "url": url, "error": str(e)}
        return None

    try:
        page = confluence.get_page_by_id(page_id, expand="body.storage")
        return {
            "title": page.get("title", ""),
            "url": url,
            "content": page.get("body", {}).get("storage", {}).get("value", ""),
            "content_text": html_to_text(page.get("body", {}).get("storage", {}).get("value", "")),
        }
    except Exception as e:
        return {"title": "Error", "url": url, "error": str(e)}


def html_to_text(html: str) -> str:
    """Simple HTML to text conversion."""
    if not html:
        return ""

    # Try using BeautifulSoup if available
    try:
        from bs4 import BeautifulSoup

        soup = BeautifulSoup(html, "html.parser")

        # Remove scripts and styles
        for script in soup(["script", "style"]):
            script.decompose()

        # Get text
        text = soup.get_text(separator="\n")

        # Clean up whitespace
        lines = (line.strip() for line in text.splitlines())
        text = "\n".join(line for line in lines if line)
        return text
    except ImportError:
        pass

    # Fallback: simple regex-based conversion
    # Remove scripts and styles
    html = re.sub(r"<script[^>]*>.*?</script>", "", html, flags=re.DOTALL | re.IGNORECASE)
    html = re.sub(r"<style[^>]*>.*?</style>", "", html, flags=re.DOTALL | re.IGNORECASE)

    # Convert some tags to text
    html = re.sub(r"<br\s*/?>", "\n", html, flags=re.IGNORECASE)
    html = re.sub(r"<p[^>]*>", "\n", html, flags=re.IGNORECASE)
    html = re.sub(r"</p>", "\n", html, flags=re.IGNORECASE)
    html = re.sub(r"<li[^>]*>", "\n• ", html, flags=re.IGNORECASE)
    html = re.sub(r"<h[1-6][^>]*>", "\n\n## ", html, flags=re.IGNORECASE)
    html = re.sub(r"</h[1-6]>", "\n", html, flags=re.IGNORECASE)

    # Remove all remaining tags
    html = re.sub(r"<[^>]+>", "", html)

    # Decode entities
    html = html.replace("&nbsp;", " ")
    html = html.replace("&lt;", "<")
    html = html.replace("&gt;", ">")
    html = html.replace("&amp;", "&")
    html = html.replace("&quot;", '"')

    # Clean up whitespace
    html = re.sub(r"\n{3,}", "\n\n", html)
    return html.strip()


def download_attachment_content(jira_client, attachment) -> Optional[str]:
    """
    Download attachment content (text-based files only).
    Returns content as string or description for binary files.
    """
    filename = attachment.filename

    # Check if it's a text-based file
    text_extensions = [
        ".txt",
        ".md",
        ".json",
        ".xml",
        ".yaml",
        ".yml",
        ".html",
        ".htm",
        ".csv",
        ".log",
        ".spec",
        ".feature",
        ".rst",
        ".adoc",
        ".textile",
    ]

    is_text = any(filename.lower().endswith(ext) for ext in text_extensions)

    if not is_text:
        return f"[Binary file: {filename}, size: {attachment.size} bytes]"

    try:
        # Download attachment content
        content = attachment.get()
        if isinstance(content, bytes):
            return content.decode("utf-8", errors="replace")
        return str(content)
    except Exception as e:
        return f"[Error downloading {filename}: {e}]"


def render_description(issue) -> str:
    """
    Render issue description to plain text.
    Handles both plain text and Atlassian Document Format (ADF).
    """
    description = issue.fields.description

    if description is None:
        return ""

    # If it's already a string (plain text or wiki markup)
    if isinstance(description, str):
        return description

    # If it's ADF (dict-like object)
    if hasattr(description, "content") or isinstance(description, dict):
        return adf_to_text(description if isinstance(description, dict) else description.raw)

    return str(description)


def adf_to_text(adf: dict) -> str:
    """Convert Atlassian Document Format to plain text."""
    if not adf or not isinstance(adf, dict):
        return ""

    result = []

    def process_node(node):
        if not isinstance(node, dict):
            return

        node_type = node.get("type", "")

        if node_type == "text":
            result.append(node.get("text", ""))
        elif node_type == "hardBreak":
            result.append("\n")
        elif node_type == "paragraph":
            for child in node.get("content", []):
                process_node(child)
            result.append("\n")
        elif node_type == "heading":
            level = node.get("attrs", {}).get("level", 1)
            result.append("#" * level + " ")
            for child in node.get("content", []):
                process_node(child)
            result.append("\n")
        elif node_type == "bulletList" or node_type == "orderedList":
            for child in node.get("content", []):
                process_node(child)
        elif node_type == "listItem":
            result.append("• ")
            for child in node.get("content", []):
                process_node(child)
        elif node_type == "codeBlock":
            result.append("\n```\n")
            for child in node.get("content", []):
                process_node(child)
            result.append("\n```\n")
        elif node_type == "inlineCard" or node_type == "blockCard":
            url = node.get("attrs", {}).get("url", "")
            if url:
                result.append(f"[Link: {url}]")
        else:
            # Process children for unknown types
            for child in node.get("content", []):
                process_node(child)

    for node in adf.get("content", []):
        process_node(node)

    return "".join(result).strip()


def fetch_jira_spec(ticket_id: str) -> dict:
    """
    Fetch specification from JIRA ticket.

    Returns dict with:
    - ticket_id: The ticket ID
    - title: Issue summary
    - description: Plain text description
    - attachments: List of attachment contents
    - confluence_pages: List of linked Confluence pages
    - remote_links: Other linked resources
    """
    config = load_config()
    jira = get_jira_client(config)

    # Fetch issue with all fields
    issue = jira.issue(ticket_id, expand="attachment,remotelink")

    # Get description
    description_text = render_description(issue)

    # Get attachments
    attachments = []
    if hasattr(issue.fields, "attachment") and issue.fields.attachment:
        for att in issue.fields.attachment:
            content = download_attachment_content(jira, att)
            attachments.append({
                "filename": att.filename,
                "content": content,
            })

    # Get remote links
    remote_links = []
    try:
        links = jira.remote_links(ticket_id)
        for link in links:
            obj = link.object
            remote_links.append({
                "title": getattr(obj, "title", ""),
                "url": getattr(obj, "url", ""),
            })
    except Exception:
        pass  # Remote links are optional

    # Extract Confluence links from description
    confluence_urls = extract_confluence_links(description_text)

    # Also check remote links for Confluence URLs
    for link in remote_links:
        url = link.get("url", "")
        if "confluence" in url.lower() or "/wiki/" in url:
            confluence_urls.append(url)

    # Check comments for Confluence links
    try:
        comments = jira.comments(ticket_id)
        for comment in comments:
            body = comment.body if isinstance(comment.body, str) else adf_to_text(comment.body.raw if hasattr(comment.body, "raw") else {})
            confluence_urls.extend(extract_confluence_links(body))
    except Exception:
        pass

    # Fetch Confluence pages (only if we have Confluence configured and have URLs)
    confluence_pages = []
    unique_urls = list(set(confluence_urls))

    if unique_urls:
        confluence_config = config.get("confluence", {})
        if confluence_config.get("url") and confluence_config.get("token"):
            try:
                confluence = get_confluence_client(config)
                for url in unique_urls:
                    page = fetch_confluence_page(confluence, url)
                    if page:
                        confluence_pages.append(page)
            except Exception as e:
                confluence_pages.append({"error": f"Failed to connect to Confluence: {e}"})
        else:
            # Confluence not configured, just list the URLs
            for url in unique_urls:
                confluence_pages.append({
                    "title": "Not fetched",
                    "url": url,
                    "error": "Confluence not configured. Set CONFLUENCE_URL and CONFLUENCE_TOKEN.",
                })

    # Build result
    jira_url = config.get("jira", {}).get("url", "")

    return {
        "ticket_id": ticket_id,
        "url": f"{jira_url}/browse/{ticket_id}",
        "title": issue.fields.summary,
        "status": issue.fields.status.name if issue.fields.status else "",
        "description": description_text,
        "attachments": attachments,
        "confluence_pages": confluence_pages,
        "remote_links": remote_links,
    }


def format_spec(spec: dict) -> str:
    """Format spec for display."""
    output = []

    output.append(f"JIRA Ticket: {spec['ticket_id']}")
    output.append(f"URL: {spec['url']}")
    output.append(f"Title: {spec['title']}")
    output.append(f"Status: {spec['status']}")

    if spec["description"]:
        output.append("\n" + "=" * 60)
        output.append("DESCRIPTION:")
        output.append("-" * 60)
        output.append(spec["description"])

    if spec["attachments"]:
        output.append("\n" + "=" * 60)
        output.append(f"ATTACHMENTS ({len(spec['attachments'])}):")
        for att in spec["attachments"]:
            output.append("-" * 60)
            output.append(f"File: {att['filename']}")
            output.append(att["content"] or "[No content]")

    if spec["confluence_pages"]:
        output.append("\n" + "=" * 60)
        output.append(f"CONFLUENCE PAGES ({len(spec['confluence_pages'])}):")
        for page in spec["confluence_pages"]:
            output.append("-" * 60)
            if page.get("error"):
                output.append(f"URL: {page.get('url', 'Unknown')}")
                output.append(f"Error: {page['error']}")
            else:
                output.append(f"Page: {page.get('title', 'Unknown')}")
                output.append(f"URL: {page.get('url', '')}")
                output.append(page.get("content_text", "") or "[No content]")

    if spec["remote_links"]:
        output.append("\n" + "=" * 60)
        output.append(f"REMOTE LINKS ({len(spec['remote_links'])}):")
        for link in spec["remote_links"]:
            output.append(f"  - {link['title']}: {link['url']}")

    return "\n".join(output)


def main():
    """CLI interface."""
    if len(sys.argv) < 2:
        print("Usage: jira_fetcher.py <ticket_id>")
        print("\nExample:")
        print("  jira_fetcher.py TEM61510-12345")
        print("\nRequired packages:")
        print("  pip install jira atlassian-python-api")
        print("\nEnvironment variables:")
        print("  JIRA_URL                   - JIRA server URL")
        print("  JIRA_TOKEN                 - JIRA API token (Personal Access Token)")
        print("  JIRA_SKIP_SSL_VERIFY       - Skip SSL verification (true/false)")
        print("")
        print("  CONFLUENCE_URL             - Confluence server URL")
        print("  CONFLUENCE_TOKEN           - Confluence API token (Personal Access Token)")
        print("  CONFLUENCE_SKIP_SSL_VERIFY - Skip SSL verification (true/false)")
        print("\nOr configure in ~/.config/git-review/jira.json")
        sys.exit(1)

    ticket_id = sys.argv[1]

    try:
        spec = fetch_jira_spec(ticket_id)
        print(format_spec(spec))
    except ImportError as e:
        print(f"Missing required package: {e}", file=sys.stderr)
        print("Install with: pip install jira atlassian-python-api", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
