# Perch

> 從任何瀏覽器（包含手機）直接操控跑在 server 上的 Claude Code AI agent。

Perch 是一個輕量的 web terminal server，讓你不需要 SSH，直接用瀏覽器開啟完整的 terminal 介面，即時看到 Claude Code 的輸出、輸入指令、設定排程，不論你在哪裡。

---

## 功能

- **完整 terminal**：基於 xterm.js，支援顏色、滾動、可點擊的 URL
- **即時串流**：所有連線共用同一個 PTY session，即時看到 Claude Code 輸出
- **手機支援**：虛擬鍵盤（Tab、Ctrl+C/D/Z、Esc、方向鍵），視窗縮放自動調整
- **三種認證模式**：無認證（內網測試）、密碼登入、mTLS 雙向憑證
- **排程器**：設定每天幾點自動送指令進 terminal
- **IP 封鎖**：TCP 層封鎖惡意 IP
- **限速**：HTTP 層限制登入/bootstrap 端點的請求頻率
- **自動重啟**：Claude Code 崩潰後自動重啟

---

## 快速開始

### 本地執行（開發用）

```bash
# 下載
git clone https://github.com/fcwu/perch.git
cd perch

# 建置前端
cd frontend && npm install && npm run build && cd ..

# 建置 Go binary
go build -o perch .

# 執行（無認證模式，僅限本地測試）
AUTH_MODE=none LISTEN_ADDR=:8443 ./perch
```

瀏覽器開啟 **`https://localhost:8443`**，接受自簽憑證警告，即可進入 terminal。

### Docker 執行（建議正式使用）

```bash
# 從 GitHub Container Registry 拉取
docker pull ghcr.io/fcwu/perch:latest

# 無認證模式（內網測試）
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=none \
  ghcr.io/fcwu/perch:latest

# 密碼模式
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=password \
  -e AUTH_PASSWORD=你的密碼 \
  -v perch-data:/app/data \
  ghcr.io/fcwu/perch:latest

# mTLS 模式（最安全，正式對外使用）
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=mtls \
  -v perch-data:/app/data \
  ghcr.io/fcwu/perch:latest
```

---

## 環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `AUTH_MODE` | `none` | 認證模式：`none` / `password` / `mtls` |
| `AUTH_PASSWORD` | — | 密碼（`AUTH_MODE=password` 時必填） |
| `LISTEN_ADDR` | `:8443` | 監聽位址，例如 `:8443` 或 `0.0.0.0:443` |
| `BLOCK_IPS` | — | 空格分隔的封鎖 IP 清單，支援 CIDR，例如 `1.2.3.4 10.0.0.0/8` |

---

## 認證模式說明

### `AUTH_MODE=none` — 無認證

無任何驗證，所有人可直接連線。**僅限內網或本地測試使用**，絕對不要暴露在公網。

### `AUTH_MODE=password` — 密碼登入

連線後需輸入密碼才能看到 terminal。密碼以 cookie session 方式儲存。

```bash
AUTH_MODE=password AUTH_PASSWORD=mysecret ./perch
```

### `AUTH_MODE=mtls` — 雙向 TLS（mTLS）

最安全的模式，瀏覽器必須安裝 client 憑證才能連線。

**首次設定流程：**

1. 啟動 server（mTLS 模式）
2. 第一次連線時，訪問 **`https://<your-server>:8443/bootstrap`** 下載 `client.p12`
3. 在手機 / 電腦安裝 `client.p12`（密碼：`perch`）
4. Bootstrap 端點自動失效（只能用一次）
5. 之後連線時，瀏覽器自動帶上 client 憑證

**Android Chrome 安裝憑證：**
- 設定 → 安全性 → 加密憑證 → 安裝憑證 → 選擇 `.p12`

**iOS Safari 安裝憑證：**
- 下載後跳出安裝提示 → 去「設定 → 一般 → VPN 與裝置管理」安裝

---

## 在手機上使用

1. 確認手機與電腦 / server 在同一網路（或 server 有公網 IP）
2. 手機 Chrome 開啟 `https://<server-ip>:8443`
3. 接受自簽憑證警告（或使用 mTLS 模式安裝憑證）
4. 畫面下方有虛擬鍵盤，點擊按鈕送出特殊按鍵

虛擬鍵盤按鍵：

| 按鈕 | 送出 |
|------|------|
| Tab | Tab 補全 |
| Ctrl+C | 中斷目前程序 |
| Ctrl+D | EOF / 登出 |
| Ctrl+Z | 暫停程序 |
| Esc | Escape |
| ↑ ↓ ← → | 方向鍵（歷史指令等） |
| ▼ | 收合鍵盤 |

---

## 排程器 API

可以設定每天特定時間自動送指令進 terminal（例如：每天早上 9 點叫 Claude 做 daily review）。

### 列出排程

```bash
curl -sk https://localhost:8443/schedule
```

### 新增排程

```bash
curl -sk -X POST https://localhost:8443/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "hour": 9,
    "minute": 0,
    "message": "幫我做今天的 daily standup 摘要",
    "repeat": true
  }'
```

`repeat: true` = 每天重複；`repeat: false` = 只執行一次。

### 刪除排程

```bash
curl -sk -X DELETE https://localhost:8443/schedule/<id>
```

排程資料存在 `schedules.json`，重啟後不遺失（Docker 需掛 volume）。

---

## Docker Volume 說明

正式使用建議掛 volume 持久化排程資料：

```bash
docker run -d \
  -p 8443:8443 \
  -e AUTH_MODE=password \
  -e AUTH_PASSWORD=mysecret \
  -v perch-data:/app \
  ghcr.io/fcwu/perch:latest
```

`/app` 目錄下會儲存 `schedules.json`。

---

## 架構說明

```
手機 Chrome
    │
    │ HTTPS / WSS
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
│               │             │
│               ▼             │
│          Claude Code        │
└─────────────────────────────┘
```

- 所有瀏覽器連線共用同一個 PTY session（多人同時看到同樣畫面）
- Claude Code 崩潰後自動重啟
- IP 封鎖在 TLS handshake 之前就丟棄連線

---

## 從原始碼建置

```bash
git clone https://github.com/fcwu/perch.git
cd perch

# 執行測試
go test ./...

# 建置前端
cd frontend
npm install
npm run build
cd ..

# 建置 binary
go build -o perch .
```

需要：Go 1.25+、Node.js 20+

---

## License

MIT
