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
RUN CGO_ENABLED=0 GOOS=linux go build -o perch .

# Stage 3: Runtime
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git nodejs npm gosu && \
    rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code

WORKDIR /app
COPY --from=builder /app/perch .
COPY claude/ /app/perch-claude/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV AUTH_MODE=none
ENV LISTEN_ADDR=:8443

EXPOSE 8443
CMD ["/entrypoint.sh"]
