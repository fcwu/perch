## Why

mTLS 認證對一般自架用戶而言設定複雜（需要 self-signed CA、client cert bootstrap、HTTPS 強制），但實際提供的安全效益可以用 Cloudflare Zero Trust 或 VPN 等外層方案取代。保留它只會增加程式碼複雜度與測試維護成本，沒有對應的使用族群支撐。

## What Changes

- **BREAKING**：移除 `AUTH_METHOD=mtls`；啟動時若設定此值將拒絕啟動並顯示明確錯誤
- 刪除 `tls.go`（TLS cert 生成工具）與 `bootstrap.go`（client P12 生成 + `/bootstrap` 路由）
- `auth.go`：移除 `mtls` case；`server.go`：移除 mtls redirect 邏輯
- `main.go`：從 `validMethods` 移除 `mtls`；移除 TLS listener 啟動區塊
- Settings UI：`AUTH_METHOD` 選項移除 `mtls`
- README：認證方式從四種縮減為三種（`none` / `password` / `gitlab`）；更新「單使用者/多使用者」說明
- 測試：刪除所有 mTLS 相關 unit test 及 e2e test case（`auth_test.go` T23/T24、`server_test.go` mtls redirect test、`tests/test-auth-modes.md` T12/T23/T24/AL14、`tests/test-auth-login-ui.md` AL14）

## Capabilities

### New Capabilities
（無）

### Modified Capabilities
- `auth-providers`：移除 `mtls` 作為有效的 `AUTH_METHOD` 值；有效值縮減為 `none` / `password` / `gitlab`

## Impact

- **Go**：`tls.go`、`bootstrap.go`（整檔刪除）；`auth.go`、`server.go`、`main.go`（局部刪除）
- **Frontend**：`SettingsPanel.tsx`（移除 mtls 選項，若有的話）
- **Tests**：`auth_test.go`、`server_test.go`（unit）；`tests/test-auth-modes.md`、`tests/test-auth-login-ui.md`（e2e）
- **Docs**：`README.md`、`DEVELOPMENT.md`
- **openspec**：`openspec/specs/auth-providers/spec.md` delta（移除 mtls scenarios）
- 無 DB schema 或 API 路由新增；`/bootstrap` 路由整個消失（**BREAKING**）
