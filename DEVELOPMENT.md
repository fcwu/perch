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

瀏覽器開啟 **`http://localhost:8080`**，會自動重導向至 `/chat`（Chat UI）；terminal 在 `http://localhost:8080/terminal`。

> Perch 使用 plain HTTP；TLS termination 交由反向代理（Cloudflare、nginx 等）處理。

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
- IP 封鎖在 TCP 層直接丟棄連線

---

## Routes

| Route | Description |
|---|---|
| `/` | Redirects to `/chat` |
| `/chat` | Main Chat UI (Open Web UI style) |
| `/chat?id=<uuid>` | Opens specific conversation |
| `/terminal` | xterm.js terminal (admin only) |
| `/admin` | Admin panel (realtime/history/analytics) |

---

## Runtime Settings

Runtime configuration is stored in `/data/settings.json` (created automatically on first run).
Settings take precedence over environment variables. To reset a setting to its env var default,
remove it from `settings.json`.

Three hot-reload tiers:
- **Immediate**: rate_limit_rpm, block_ips, auth.password, agent.args, workspace.*
- **Next session**: agent.runtime, log.format
- **Restart required**: auth.method, perch_mode, discord/telegram tokens, gitlab.*

## Volume Requirements

`/data` must be a persistent volume (conversation history + settings stored there):
```yaml
volumes:
  - /path/to/data:/data
```

---

## 測試案例

詳見 `tests/test-*.md`（依功能拆分）。

---

## 已知限制

### AL23 — 登出後 server-side session 未撤銷（password 模式）

`POST /api/logout` 會清除瀏覽器端 cookie（`Max-Age=0, SameSite=Lax`），一般瀏覽器使用下行為正確。但 `AuthMiddleware.sessions` map 不會移除已登出的 token，若攻擊者事先取得 session token 原始值（例如 curl cookie jar），仍可在登出後繼續使用。

**影響範圍**：僅 `AUTH_METHOD=password` 模式；GitLab OAuth 使用有時效的 JWT，不受此限制影響。

**未修原因**：perch 定位為個人工具，實際風險低；瀏覽器用戶不受影響。

**修法**：`AuthMiddleware` 加 `DeleteSession(token string)` 方法，在 `handleLogout` 讀出 `session` cookie 後呼叫。
