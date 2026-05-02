## 1. Go：移除 mTLS 核心程式碼

- [x] 1.1 刪除 `tls.go`（整檔）
- [x] 1.2 刪除 `bootstrap.go`（整檔）
- [x] 1.3 `auth.go`：移除 `mtls` case 及相關邏輯
- [x] 1.4 `server.go`：移除 mtls redirect to `/bootstrap` 邏輯
- [x] 1.5 `main.go`：從 `validMethods` 移除 `"mtls"`；移除 TLS listener 啟動區塊（`if authMethod == "mtls" {...}`）；在無效 auth method 錯誤訊息後加入對 mtls 的明確拒絕提示
- [x] 1.6 `gitlab_auth.go`：移除 `authMethod` 欄位或 comment 中的 `mtls` 標注
- [x] 1.7 `go build ./...` 確認編譯通過

## 2. Frontend：移除 mtls 選項

- [x] 2.1 `SettingsPanel.tsx`：在 `AUTH_METHOD` 選項清單中移除 `mtls`（如有）

## 3. Tests：刪除 mTLS 測試

- [x] 3.1 `auth_test.go`：刪除 T23（無 client cert 302 redirect）與 T24（`/bootstrap` 不需 cert）兩個 test function
- [x] 3.2 `server_test.go`：刪除 mtls redirect 相關 test function
- [x] 3.3 `go test ./...` 確認全部通過

## 4. e2e Test Docs：更新測試文件

- [x] 4.1 `tests/test-auth-modes.md`：刪除 T12（mTLS Bootstrap 流程）、T23、T24 及 mTLS bug 紀錄段落
- [x] 4.2 `tests/test-auth-login-ui.md`：刪除 AL14（AUTH_METHOD=mtls 自動生成憑證）段落

## 5. Docs & Config

- [x] 5.1 `README.md`：將「四種認證方式」改為三種；移除 `mtls` 在環境變數表的描述、`### mtls` 說明段落、目錄連結；在 Breaking Changes 或 Migration 加 mtls 移除提示
- [x] 5.2 `DEVELOPMENT.md`：移除 mTLS 架構圖及相關說明
- [x] 5.3 `openspec/specs/auth-providers/spec.md`：套用 delta（修改 accepted values、移除 mtls scenarios）
