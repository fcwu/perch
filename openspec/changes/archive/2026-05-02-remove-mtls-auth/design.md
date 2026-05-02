## Context

Perch 目前支援四種 `AUTH_METHOD`：`none`、`password`、`mtls`、`gitlab`。mTLS 是唯一需要 HTTPS listener 的模式，依賴 `tls.go`（self-signed cert 生成）與 `bootstrap.go`（client P12 生成、`/bootstrap` 路由）兩個獨立模組。實際上沒有已知的生產使用者依賴 mTLS；外層安全（Cloudflare Zero Trust、VPN）已能滿足同等需求。

## Goals / Non-Goals

**Goals:**
- 刪除 `tls.go`、`bootstrap.go` 兩個整檔
- 從 `auth.go`、`server.go`、`main.go`、`SettingsPanel.tsx` 移除所有 mtls 分支
- 刪除對應 unit test 與 e2e test case
- 更新 README / DEVELOPMENT.md / openspec spec

**Non-Goals:**
- 移除一般 HTTPS 支援（perch 未來若要加 TLS termination，另立 change）
- 改動 `none` / `password` / `gitlab` 任何現有行為

## Decisions

### D1：啟動時若 `AUTH_METHOD=mtls` 直接 `os.Exit(1)` 並印清楚錯誤

理由：靜默降級（自動退為 `none`）會讓用戶誤以為 mTLS 還在運作。明確拒絕啟動比偷偷改模式安全。

### D2：`/bootstrap` 路由整個移除，不保留 404 stub

理由：沒有流量依賴此路徑（mTLS 才會用到）。留 stub 只會誤導未來讀者。

### D3：`tls.go` 與 `bootstrap.go` 整檔刪除，不保留任何 stub

理由：兩個檔案所有函式都只服務 mTLS，刪整檔比留空殼乾淨。

## Risks / Trade-offs

- **BREAKING**：設定 `AUTH_METHOD=mtls` 的容器升級後會拒絕啟動 → Mitigation：README migration note 說明改用 `none` 或 `password`
- 無 DB migration、無 API schema 變更；風險最小化

## Migration Plan

1. 升級映像前確認 `AUTH_METHOD` 不是 `mtls`；若是，改為 `none`（內網）或 `password`（密碼保護）
2. `/bootstrap` 路由消失，書籤或 bookmark 此 URL 的用戶會收到 404
3. 無 rollback 複雜度：舊映像仍支援 mtls；新映像不支援
