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
    ca-certificates curl git jq nodejs npm gosu && \
    rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code
RUN curl -fsSL https://api.github.com/repos/anomalyco/opencode/releases/latest | \
    jq -r '(.assets[] | select(.name=="opencode-linux-arm64.tar.gz") | .browser_download_url)' | \
    xargs -I {} sh -lc 'tmp=$(mktemp -d) && curl -fsSL "{}" -o "$tmp/opencode.tgz" && tar -xzf "$tmp/opencode.tgz" -C /usr/local/bin && chmod +x /usr/local/bin/opencode && rm -rf "$tmp"'

WORKDIR /app
COPY --from=builder /app/perch .
COPY claude/ /app/perch-claude/
COPY .opencode/ /app/perch-opencode/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV AUTH_MODE=none

EXPOSE 8080
CMD ["/entrypoint.sh"]
