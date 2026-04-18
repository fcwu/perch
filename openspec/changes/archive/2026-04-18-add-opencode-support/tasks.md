## 1. Runtime Selection Foundation

- [x] 1.1 定義 agent runtime 設定模型（例如 `AGENT_RUNTIME`、runtime-specific args 讀取規則）並在啟動時驗證只接受支援的 runtime 值
- [x] 1.2 新增 runtime descriptor / abstraction，封裝命令名稱、預設參數、額外環境變數、project config 路徑與 bundled asset 來源
- [x] 1.3 更新主 PTY 啟動流程，改為透過 runtime abstraction 啟動預設 agent，而不是直接硬編碼 `claude`

## 2. PTY Session Integration

- [x] 2.1 更新 Discord session PTY 啟動流程，讓 `newDiscordSession` 使用目前選定的 runtime 建立 agent process
- [x] 2.2 確認 scheduler 寫入 main PTY 與 `discord:<channelID>` PTY 時，實際對應到的 session 都由同一個 active runtime 建立
- [x] 2.3 補上 runtime selection 的單元測試，覆蓋預設 Claude、OpenCode 啟動與非法設定失敗情境

## 3. Runtime Asset Sync And Image Support

- [x] 3.1 更新 `Dockerfile`，讓 runtime image 具備 OpenCode 可執行檔與所需支援資產，同時保留 Claude 既有安裝路徑
- [x] 3.2 將 `entrypoint.sh` 改為 runtime-aware，同步 Claude 或 OpenCode 的 project-level config assets 到對應 workspace 目錄
- [x] 3.3 擴充或泛化現有 settings merge / plugin sync 流程，使其可依 active runtime 寫入對應設定檔，而不是只處理 `.claude/settings.json`

## 4. Hook And IM Parity

- [x] 4.1 實作 OpenCode runtime 的 callback / hook integration，讓 Perch 在 OpenCode 執行期間仍能收到 Discord/IM 所需的 progress 或 completion 事件
- [x] 4.2 驗證 Discord 訊息在 OpenCode runtime 下可正常寫入 PTY，並在完成後將結果回傳到原 channel 或 reply thread
- [x] 4.3 驗證 scheduler 目標為 Discord session 時，在 OpenCode runtime 下仍會先送 header，再送完成回應

## 5. Documentation And Verification

- [x] 5.1 更新 `README.md`：說明如何選擇 `claude` / `opencode` runtime、各自使用的 args 與必要掛載或設定
- [x] 5.2 更新 `docs/test-cases.md`，新增 OpenCode runtime smoke tests 與 Claude/OpenCode parity 測試案例
- [x] 5.3 執行並記錄核心驗證：預設 Claude 不回歸、OpenCode 可啟動、Discord 流程可用、scheduler 流程可用

## Notes

- 本地 smoke test 發現 OpenCode install script 會把 binary 放在 `/root/.opencode/bin/opencode`，image 必須顯式連結到 `/usr/local/bin/opencode` 才能讓 Perch 透過 `exec.Command("opencode", ...)` 啟動。
- 驗證記錄：
  - `go test ./...` 通過
  - 現有本地 container `mykb`（`perch:local`，port `8081`）正常提供 `/` 與 `/sessions`，驗證預設 Claude 路徑未回歸
  - 臨時 container `perch-opencode-smoke` 以 `AGENT_RUNTIME=opencode` 啟動成功，`command -v opencode` 回傳 `/usr/local/bin/opencode`，且 `opencode --help` exit code 為 `0`
