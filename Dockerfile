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
    ca-certificates curl git jq gosu && \
    curl -fsSL https://deb.nodesource.com/setup_24.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp
RUN curl -fsSL https://api.github.com/repos/anomalyco/opencode/releases/latest | \
    jq -r '(.assets[] | select(.name=="opencode-linux-arm64.tar.gz") | .browser_download_url)' | \
    xargs -I {} sh -lc 'tmp=$(mktemp -d) && curl -fsSL "{}" -o "$tmp/opencode.tgz" && tar -xzf "$tmp/opencode.tgz" -C /usr/local/bin && chmod +x /usr/local/bin/opencode && rm -rf "$tmp"'

WORKDIR /app
COPY --from=builder /app/perch .
COPY claude/ /app/perch-claude/
COPY opencode/ /app/perch-opencode/
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
