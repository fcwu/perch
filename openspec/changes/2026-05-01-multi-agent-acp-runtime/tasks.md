## 0. Pre-flight

- [x] 0.1 實測 `opencode acp` stdio JSON-RPC：sst/opencode 1.14.30 正常回 `initialize` response；**stdout 預設帶 INFO logs**，必須 `--log-level WARN` 才能跟 ACP 協定共存。`promptCapabilities.image: true`、`authMethods: [opencode-login]`
- [x] 0.2 實測 `session/new` payload：perch 既有 `{cwd, mcpServers:[]}` ✅ accepts；`session/set_mode "bypassPermissions"` ❌ opencode 只認 `build`/`plan` mode（acp_process.go:189 已 graceful warning-and-continue，default `build` mode 也跑 tools）。免費 `opencode/*` 模型 credential-less，付費需 `opencode auth login` 互動式登入或 mount `~/.local/share/opencode/auth.json`
- [x] 0.3 Q1（不加 codex） / Q2（pre-flight 已驗）已決；Q3-Q5 留 design.md 內以 D6 / startup log 為準，本 change 不另拍板

## 1. Runtime abstraction：擴 ACP 欄位

- [x] 1.1 `runtime.go::AgentRuntime` 加 `ACPExecutable string` + `ACPArgs []string`
- [x] 1.2 `loadAgentRuntime`：`claude` → `ACPExecutable:"claude-agent-acp"`；`opencode` → `ACPExecutable:"opencode", ACPArgs:["acp","--log-level","WARN"]`
- [x] 1.3 `runtime_test.go::TestLoadAgentRuntime_ACPFields` 三個 sub-case（default-claude / claude-explicit / opencode）全 PASS

## 2. ACP path：吃 runtime 而非 hardcode

- [x] 2.1 `acp_process.go::NewACPProcess(executable, args, workdir, logger)` 接受 args slice；`exec.Command(p.executable, p.args...)`
- [x] 2.2 `ACP_EXECUTABLE` env var 仍可覆蓋 executable；新增 `ACP_EXECUTABLE_ARGS`（JSON array）覆蓋 args；invalid JSON warns 並沿用 caller args
- [x] 2.3 `acp_session_pool.go::newACPSessionPool(executable, args, workdir, logger)` 接 args 並 forward 給 `NewACPProcess`
- [x] 2.4 `chat_api_acp.go::newACPUserSessionManager(runtime, ...)` + `im_telegram.go::newTelegramAdapter(runtime, ...)` 接受 runtime；pool 收 `runtime.ACPExecutable + runtime.ACPArgs`
- [x] 2.5 `im_discord.go`：移除 dead `acpExecutable` 欄位；`newDiscordSession(runtime, channelID, workdir, logger)` 直接走 runtime ACP path

## 3. main.go：wire runtime 進去

- [x] 3.1 `srv.chatSessions = newACPUserSessionManager(runtime, workdir, store, adminHub, logger.Logger)`
- [x] 3.2 `im.addAdapter(newTelegramAdapter(runtime, telegramToken, chatID, workdir, logger.Logger))`；Discord 已持有 runtime（既有）

## 4. Dockerfile：multi-arch + sst/opencode

- [x] 4.1 換 `anomalyco/opencode` → `sst/opencode` GitHub releases
- [x] 4.2 `dpkg --print-architecture`：amd64 → `opencode-linux-x64.tar.gz`、arm64 → `opencode-linux-arm64.tar.gz`、其他 → `exit 1`
- [ ] 4.3 build image 在 amd64 host 確認 `docker exec ... opencode --version` 不再 EXEC error — 留 phase 6 QA 驗
- [ ] 4.4 build image 在 arm64 host（QNAP / Graviton）— 留後續驗（不 block 本 change）

## 5. Settings UI / 文件

- [x] 5.1 `SettingsPanel.tsx` 既有 RadioGroup 沒 description text，無需改動（沒誤導文字可砍）
- [x] 5.2 README.md「Agent Runtime」段補 「runtime 影響範圍」note + 「OpenCode 額外注意」段（免費 vs 付費、auth login 流程、mode 差異 + 對應行為）
- [x] 5.3 README.md「Advanced overrides」表格涵蓋 `ACP_EXECUTABLE` + `ACP_EXECUTABLE_ARGS`

## 6. 測試

- [x] 6.1 `tests/test-multi-agent-runtime.md` 建立：MR01（claude default baseline）/ MR02（opencode subprocess switch）/ MR03（opencode + image upload）/ MR04（ACP_EXECUTABLE env override precedence）+ Sanity 段
- [ ] 6.2 全套 QA cycle 跑 MR01-04 + MT12 / T55-multi / T56 / T19 / CU01 sanity
- [ ] 6.3 對比 `tests/test-report-2026-05-01-image-upload-round2-1022.md` 確認 claude path 無 regression

## 7. 結束條件

- [ ] 7.1 全套 QA cycle zero FAIL / zero env-fix-by-qa SKIP（OpenCode image incompatibility 算 needs-user-action 不算 env-fix）
- [ ] 7.2 README、Settings UI、code 三方一致
- [ ] 7.3 archive 完成
