#!/usr/bin/env python3
"""
Mock GitLab OAuth server for local QA.

Replaces sauron.qnap.com (which requires Azure AD SSO) with a stub that:
  - GET  /oauth/authorize → 302 redirect_uri?code=MOCK_CODE&state=<state>
  - POST /oauth/token     → returns mock access_token
  - GET  /api/v4/user     → returns user profile (selected via Bearer token)

Two pre-canned users:
  - access_token=token-dorowu     → id=628, username=dorowu
  - access_token=token-testuser2  → id=999, username=testuser2

By default issues `token-dorowu`. Override with ?user=testuser2 on /oauth/authorize
to get `token-testuser2` back from /oauth/token.

Designed to be drop-in for perch's GitLabAuthProvider:

  GITLAB_URL=http://localhost:18098 \
  GITLAB_CLIENT_ID=mock GITLAB_CLIENT_SECRET=mock \
  GITLAB_REDIRECT_URI=http://localhost:18099/auth/callback \
  AUTH_METHOD=gitlab PERCH_MODE=multi \
  COOKIE_SECRET=$(openssl rand -hex 32) \
  ./perch
"""
from __future__ import annotations

import argparse
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlencode, urlparse

USERS = {
    "token-dorowu": {
        "id": 628,
        "username": "dorowu",
        "name": "Doro Wu",
        "email": "fcwu.tw@gmail.com",
        "state": "active",
    },
    "token-testuser2": {
        "id": 999,
        "username": "testuser2",
        "name": "Test User 2",
        "email": "testuser2@example.com",
        "state": "active",
    },
}

# Map ?user= query param to the access_token we'll issue.
USER_TO_TOKEN = {
    "dorowu": "token-dorowu",
    "testuser2": "token-testuser2",
}

# Holds the next access_token to issue at /oauth/token, keyed by code.
# Codes are static ("MOCK_CODE_<user>") so this is just a deterministic lookup.
PENDING_CODES = {
    "MOCK_CODE_dorowu": "token-dorowu",
    "MOCK_CODE_testuser2": "token-testuser2",
}


class Handler(BaseHTTPRequestHandler):
    server_version = "MockGitLab/1.0"

    def log_message(self, fmt, *args):
        sys.stderr.write("[mock-gitlab] %s - %s\n" % (self.address_string(), fmt % args))

    def _json(self, status: int, body: dict) -> None:
        payload = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def _redirect(self, location: str) -> None:
        self.send_response(302)
        self.send_header("Location", location)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_GET(self):  # noqa: N802 (BaseHTTPRequestHandler API)
        url = urlparse(self.path)
        if url.path == "/oauth/authorize":
            return self._handle_authorize(url)
        if url.path == "/api/v4/user":
            return self._handle_user()
        if url.path == "/healthz":
            return self._json(200, {"ok": True})
        self._json(404, {"error": "not found", "path": url.path})

    def do_POST(self):  # noqa: N802
        url = urlparse(self.path)
        if url.path == "/oauth/token":
            return self._handle_token()
        self._json(404, {"error": "not found", "path": url.path})

    def _handle_authorize(self, url) -> None:
        params = parse_qs(url.query)
        redirect_uri = (params.get("redirect_uri") or [""])[0]
        state = (params.get("state") or [""])[0]
        user = (params.get("user") or ["dorowu"])[0]
        if not redirect_uri:
            return self._json(400, {"error": "missing redirect_uri"})
        if user not in USER_TO_TOKEN:
            return self._json(400, {"error": "unknown user", "user": user})
        code = f"MOCK_CODE_{user}"
        sep = "&" if "?" in redirect_uri else "?"
        loc = f"{redirect_uri}{sep}{urlencode({'code': code, 'state': state})}"
        self._redirect(loc)

    def _handle_token(self) -> None:
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length).decode("utf-8") if length else ""
        ctype = (self.headers.get("Content-Type") or "").split(";")[0].strip()
        if ctype == "application/json":
            try:
                body = json.loads(raw or "{}")
            except json.JSONDecodeError:
                return self._json(400, {"error": "invalid json"})
        else:
            body = {k: v[0] for k, v in parse_qs(raw).items()}
        code = body.get("code", "")
        token = PENDING_CODES.get(code)
        if not token:
            return self._json(400, {"error": "invalid_grant", "code": code})
        self._json(
            200,
            {
                "access_token": token,
                "token_type": "Bearer",
                "expires_in": 7200,
                "refresh_token": f"refresh-{token}",
                "scope": "read_user api",
                "created_at": 1700000000,
            },
        )

    def _handle_user(self) -> None:
        auth = self.headers.get("Authorization", "")
        if not auth.lower().startswith("bearer "):
            return self._json(401, {"error": "missing bearer token"})
        token = auth.split(None, 1)[1].strip()
        user = USERS.get(token)
        if not user:
            return self._json(401, {"error": "unknown token", "token": token})
        self._json(200, user)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18098)
    args = parser.parse_args()

    httpd = HTTPServer((args.host, args.port), Handler)
    sys.stderr.write(f"[mock-gitlab] listening on http://{args.host}:{args.port}\n")
    sys.stderr.write(
        "[mock-gitlab] users: "
        + ", ".join(f"{u} (id={USERS[t]['id']})" for u, t in USER_TO_TOKEN.items())
        + "\n"
    )
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        sys.stderr.write("[mock-gitlab] shutting down\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
