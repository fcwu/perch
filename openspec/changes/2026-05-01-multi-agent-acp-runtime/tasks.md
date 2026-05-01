## 0. Pre-flight

- [ ] 0.1 實測 `opencode acp` 在 image 內可啟動：`docker exec ... sh -lc 'echo {} | opencode acp'` 能進入 ACP JSON-RPC 模式並接受 `initialize`。記下 opencode 版本與 ACP capability flags
- [ ] 0.2 實測 `new_session` 對 OpenCode 的接受度（D3 假設驗證）：送 `{"permissionMode":"bypassPermissions","workspace_path":"/workspace"}` 看是否 accept；若 reject 記下 minimal viable params 並回頭修 design
- [ ] 0.3 解 Open Questions Q2-Q5（Q1 已決）

## 1. Runtime abstraction：擴 ACP 欄位

- [ ] 1.1 `runtime.go::AgentRuntime` 加 `ACPExecutable string` + `ACPArgs []string`
- [ ] 1.2 `loadAgentRuntime`：`claude` case 填 `ACPExecutable:"claude-agent-acp"`、`opencode` case 填 `ACPExecutable:"opencode", ACPArgs:[]string{"acp"}`
- [ ] 1.3 `runtime_test.go` 新增 case：`TestLoadAgentRuntime_ACPFields` 驗 `claude` / `opencode` 兩個 runtime 各自正確；missing AGENT_RUNTIME 預設 claude

## 2. ACP path：吃 runtime 而非 hardcode

- [ ] 2.1 `acp_process.go::NewACPProcess` 簽名加 `args []string`，`exec.Command(executable, args...)`
- [ ] 2.2 `ACP_EXECUTABLE` env var 改成 override（在 `NewACPProcess` 開頭：env 設了則覆蓋 args 與 executable 兩者，分別由 `ACP_EXECUTABLE` 與新增的 `ACP_EXECUTABLE_ARGS` 控制；若只設 executable，args 沿用 caller 傳入）
- [ ] 2.3 `acp_session_pool.go::newACPSessionPool` 簽名加 `extraArgs []string`，forward 給 `NewACPProcess`
- [ ] 2.4 `chat_api_acp.go::newACPUserSessionManager` 簽名加 `runtime AgentRuntime`；起 pool 時傳 `runtime.ACPExecutable, runtime.ACPArgs`
- [ ] 2.5 `im_discord.go::DiscordSessionManager` 把 `acpExecutable string` 換成持有 `runtime AgentRuntime`（`runtime` 已在 struct 內），起 session 時把兩個欄位傳給 pool

## 3. main.go：wire runtime 進去

- [ ] 3.1 `main.go` 把 `runtime` 傳進 `newACPUserSessionManager(workdir, store, adminHub, logger, runtime)`
- [ ] 3.2 Discord/Telegram constructor 已經吃 runtime — 確認沒漏

## 4. Dockerfile：multi-arch + sst/opencode

- [ ] 4.1 把 `anomalyco/opencode` 換成 `sst/opencode` GitHub releases
- [ ] 4.2 `dpkg --print-architecture` 對 amd64 → `linux-x64.tar.gz`，arm64 → `linux-arm64.tar.gz`，其他 → `exit 1`
- [ ] 4.3 build image 在 amd64 host 確認 `docker exec ... opencode --version` 不再 EXEC error
- [ ] 4.4 build image 在 arm64 host（QNAP / Graviton 模擬）也試一次

## 5. Settings UI / 文件

- [ ] 5.1 `frontend/src/SettingsPanel.tsx` Agent Runtime 描述刪除「OpenCode 限 web /ws」誤導文字（如果有；目前是 RadioGroup 沒 description，可能只需 README 改）
- [ ] 5.2 README.md「Agent Runtime」段落補 bullet：「`AGENT_RUNTIME=opencode` 後 chat-API、Discord、Telegram 也會跟著切；切換需重啟」
- [ ] 5.3 README.md `ACP_EXECUTABLE` / `ACP_EXECUTABLE_ARGS` 在「環境變數」段下「可在 Settings UI 調整」**之外**新增「Advanced overrides」小段（dev 才用）

## 6. 測試

- [ ] 6.1 撰寫 `tests/test-multi-agent-runtime.md`：
  - **MR01**：`AGENT_RUNTIME` 未設 → 預設 claude；chat-API + Discord 走 `claude-agent-acp`（同既有 baseline）
  - **MR02**：`AGENT_RUNTIME=opencode` 重啟 → web `/ws` PTY 跑 `opencode` CLI；chat-API ACP subprocess 是 `opencode acp`
  - **MR03**：`AGENT_RUNTIME=opencode` 下跑 CU01（純文字 + image）→ OpenCode ACP 接受並回應（接受度由 phase 0 決定的 baseline；若 OpenCode image 不接受可在本 case 標 SKIP-as-needs-user-action）
  - **MR04**：`ACP_EXECUTABLE=claude-agent-acp` + `AGENT_RUNTIME=opencode` 同設 → executable override 生效，subprocess 是 claude（驗 D6 precedence）
- [ ] 6.2 全套 QA cycle 跑：MR01-04（新功能）+ MT12 / T55-multi / T56 / T19 / CU01 sanity（regression）
- [ ] 6.3 對比 archive 後的 `tests/test-report-2026-05-01-image-upload-round2-1022.md` 確認 claude path 無 regression

## 7. 結束條件

- [ ] 7.1 全套 QA cycle zero FAIL / zero env-fix-by-qa SKIP（OpenCode image incompatibility 算 needs-user-action 不算 env-fix）
- [ ] 7.2 README、Settings UI、code 三方一致
- [ ] 7.3 archive 完成
