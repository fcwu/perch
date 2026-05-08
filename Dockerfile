# Stage 1: Build frontend
FROM node:20-slim AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o perch .

# Stage 3: Runtime
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git jq gosu \
    fonts-noto-cjk fonts-noto-cjk-extra \
    python3 && \
    curl -fsSL https://deb.nodesource.com/setup_24.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    rm -rf /var/lib/apt/lists/*
# codex-acp ships per-platform native binaries via optionalDependencies.
# npm sometimes skips those silently; explicitly install the matching arch
# package so the bin shim can resolve the binary at runtime.
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in \
      amd64) CODEX_PLATFORM_PKG="@zed-industries/codex-acp-linux-x64" ;; \
      arm64) CODEX_PLATFORM_PKG="@zed-industries/codex-acp-linux-arm64" ;; \
      *) echo "unsupported architecture for codex-acp: $ARCH" && exit 1 ;; \
    esac && \
    npm install -g \
      @anthropic-ai/claude-code \
      @agentclientprotocol/claude-agent-acp \
      @openai/codex \
      @zed-industries/codex-acp \
      "$CODEX_PLATFORM_PKG" \
      @playwright/mcp
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in \
      amd64) OC_ASSET="opencode-linux-x64.tar.gz" ;; \
      arm64) OC_ASSET="opencode-linux-arm64.tar.gz" ;; \
      *) echo "unsupported architecture for opencode: $ARCH" && exit 1 ;; \
    esac && \
    curl -fsSL https://api.github.com/repos/sst/opencode/releases/latest | \
    jq -r --arg n "$OC_ASSET" '(.assets[] | select(.name==$n) | .browser_download_url)' | \
    xargs -I {} sh -lc 'tmp=$(mktemp -d) && curl -fsSL "{}" -o "$tmp/opencode.tgz" && tar -xzf "$tmp/opencode.tgz" -C /usr/local/bin && chmod +x /usr/local/bin/opencode && rm -rf "$tmp"'
# Install Playwright-managed Chromium binary and system dependencies (~270MB)
# Use /opt so non-root users (PUID) can access the binary at runtime
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright
RUN npx playwright install --with-deps chromium && chmod -R a+rX /opt/ms-playwright

WORKDIR /app
COPY --from=builder /app/perch .
COPY claude/ /app/perch-claude/
COPY opencode/ /app/perch-opencode/
COPY codex/ /app/perch-codex/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# SQLite DB and persistent data — mount a volume to preserve across restarts:
# -v /your/data:/data
VOLUME ["/data"]
# GitLab OAuth for Chat UI — inject via -e at runtime:
# -e GITLAB_URL=https://gitlab.example.com
# -e GITLAB_CLIENT_ID=your-client-id
# -e GITLAB_CLIENT_SECRET=your-client-secret
# -e GITLAB_REDIRECT_URI=https://perch.example.com/auth/callback
# -e COOKIE_SECRET=$(openssl rand -hex 32)

EXPOSE 8080
CMD ["/entrypoint.sh"]
