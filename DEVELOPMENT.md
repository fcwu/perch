# Perch — 開發指南

---

## 本地執行（開發用）

```bash
git clone https://github.com/fcwu/perch.git
cd perch

# 建置前端
cd frontend && npm install && npm run build && cd ..

# 建置 Go binary
go build -o perch .

# 執行（無認證模式，僅限本地測試）
AUTH_MODE=none LISTEN_ADDR=:8080 ./perch
```

瀏覽器開啟 **`http://localhost:8080`**，即可進入 terminal。

> `AUTH_MODE=none` 和 `AUTH_MODE=password` 使用 plain HTTP；只有 `AUTH_MODE=mtls` 才使用 HTTPS。

---

## 執行測試

```bash
go test ./...
```

需要：Go 1.25+、Node.js 20+

---

## 架構說明

```
手機 Chrome / Discord / Telegram
    │
    │ HTTPS / WSS / Bot API
    ▼
┌─────────────────────────────┐
│  Perch (Go binary)          │
│                             │
│  IP Block (TCP 層)          │
│  TLS / mTLS                 │
│  Auth Middleware             │
│  Rate Limiter               │
│                             │
│  WebSocket ──► PTY          │
│  Scheduler ──► PTY          │
│  IM Bot    ──► PTY          │
│               │             │
│               ▼             │
│          Claude Code        │
│               │             │
│               ▼             │
│          Claude Hook        │
│               │             │
│               ▼             │
│  POST /hook ──► IM Bot      │
└─────────────────────────────┘
```

- 所有瀏覽器連線共用同一個 PTY session（多人同時看到同樣畫面）
- Claude Code 崩潰後自動重啟
- IP 封鎖在 TLS handshake 之前就丟棄連線

---

## 測試案例

詳見 [docs/test-cases.md](test-cases.md)。
