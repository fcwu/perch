## 0. Pre-flight

- [x] 0.1 npm registry 確認 `@zed-industries/codex-acp@0.12.0`、`bin: codex-acp`、`optionalDependencies` 涵蓋 linux-x64 + linux-arm64 + darwin/win
- [x] 0.2 本機 `npm install -g @zed-industries/codex-acp @zed-industries/codex-acp-linux-x64`（npm 在 strict mode 下沒自動拉 optionalDependency；deploy 文件需 cover），`echo initialize` 確認 stdio 乾淨（logs 走 stderr）、`promptCapabilities.image: true`、`agentInfo.version: 0.12.0`
- [x] 0.3 `session/new {cwd, mcpServers:[]}` ✅ 接受並回 `sessionId / modes / models / configOptions`；codex modes 是 `read-only`(default)/`auto`/`full-access`（不是 claude 的 `bypassPermissions`），perch 既有 set_mode call 會 error，走既有 graceful warning-and-continue（不需改 code）。**副作用：codex 預設留在 `read-only`，本 change 不接 runtime-aware mode mapping**
- [x] 0.4 design.md 已更新：Q1/Q2 解決，加 `Phase 0 pre-flight` 段、加「對 perch 的影響」表、補 Q5/Q6 處理

## 1. Runtime abstraction：加 codex case + 修 ACP client 兩個 cross-runtime bug

- [x] 1.1 `runtime.go::loadAgentRuntime` 加 `case "codex"`：`Name=codex, Command=codex, ArgsEnv=CODEX_ARGS, ProjectConfigDir=.codex, ProjectConfigFile=config.toml, AssetDir=/app/perch-codex, SupportsHooks=false, ACPExecutable=codex-acp, ACPArgs=nil`
- [x] 1.2 `runtime_test.go::TestLoadAgentRuntime_ACPFields` 加 codex sub-case；新增 `TestAgentRuntimeCanSelectCodex` 驗整組欄位（ProjectConfigDir / AssetDir / CODEX_ARGS）
- [x] 1.3 `RunAgent` / `MainArgs` / `SessionArgs` 對 codex 走預設分支（不需特例分支；本 change 不接 web `/ws` PTY）— 沒改任何方法
- [x] 1.4 **In-scope fix**：`acp_process.go::acpMsg.ID` 從 `*int64` 改為 `json.RawMessage`，dispatcher 讀 string ID（codex UUID）+ int ID（perch outbound）兩種；`call()` marshal int ID 進 RawMessage；test helpers 加 `rawIDToInt64`
- [x] 1.5 **In-scope fix**：`acp_process.go::pickAutoApproveOption(params)` 動態挑 optionId（claude=`bypassPermissions`、codex=`approved`）；hardcode 改 dynamic
- [x] 1.6 `acp_process_test.go::TestPickAutoApproveOption_RuntimeShapes` 6 個 sub-case：claude legacy、codex 三選項、only allow_always、only reject、unknown kind、empty params
- [x] 1.7 `go test ./...` 全 PASS（含既有 ACP test 套件 + IM Discord ACP 套件）

## 2. Dockerfile：image 預裝 codex-acp + interactive codex

- [x] 2.1 Dockerfile npm install 段加 `@zed-industries/codex-acp` + `@openai/codex`（互動式 CLI；給 web `/ws` PTY `Command: "codex"`） + 顯式 platform package（`@zed-industries/codex-acp-linux-x64`/`-arm64`）；理由：(a) PTY 路徑需要 `codex` binary 否則 restart loop；(b) npm 有時不自動拉 optionalDependency
- [x] 2.2 `codex/` placeholder 建立（`.gitkeep` + `skills/.gitkeep`）；`COPY codex/ /app/perch-codex/` 加進 Dockerfile
- [x] 2.3 entrypoint.sh 加 `AGENT_RUNTIME=codex` 分支：seed `/app/perch-codex/skills/` → `$WORKDIR/.codex/skills/`，chown to PUID
- [x] 2.4 build image 在 amd64 host PASS：`docker run --rm perch:local-test sh -c 'which codex-acp; codex-acp --help'` 回 `/usr/bin/codex-acp` + 正常 help（QA report `tests/test-report-2026-05-01-codex-support-1506.md`）
- [ ] 2.5 build image 在 arm64 host（QNAP / Graviton）— 留 follow-up 驗

## 3. Settings UI

- [x] 3.1 `frontend/src/SettingsPanel.tsx`：`agent.runtime` RadioGroup options 從 `['claude','opencode']` → `['claude','opencode','codex']`
- [x] 3.2 `npm run build` PASS（dist/assets/index-*.js 587KB）
- [ ] 3.3 手動驗：Settings 頁能看到 Codex 選項；選 Codex + Save & Restart → backend 收 `AGENT_RUNTIME=codex` 且重啟後 startup log 印 `runtime=codex` — **留 QA 階段**

## 4. 文件 / .env

- [x] 4.1 README.md `AGENT_RUNTIME` 列項從 `claude / opencode` 改成 `claude / opencode / codex`；`.env.example` repo 沒有此檔，pass
- [x] 4.2 README.md「Agent Runtime」段補「**Codex 額外注意**」小節：auth (`OPENAI_API_KEY`/`CODEX_API_KEY`)、`codex-acp` 是 Zed wrapper（非 `@openai/codex`）、mode 預設 `read-only` + 限制、bad key 非 fail-fast、web `/ws` PTY 不接 codex 互動式 CLI
- [x] 4.3 README.md「Advanced overrides」表格不變（`ACP_EXECUTABLE / ACP_EXECUTABLE_ARGS` 對 codex 同樣生效）

## 5. 測試

- [x] 5.1 `tests/test-codex-runtime.md` 建立：CX01 / CX02 / CX03 / CX04 + Sanity 段
- [x] 5.2 全套 e2e cycle 已跑：`tests/test-report-2026-05-01-codex-support-1506.md`
  - CX01 PASS（codex-acp spawn、prompt 串回 PINEAPPLE / KIWI、set_mode warning 預期）
  - CX02 **PASS upgraded from NEEDS-USER-ACTION**（codex 在 read-only mode 仍透過 request_permission 跑工具，perch dynamic optionId picker 自動核准）
  - CX03 **PASS better than predicted**（codex-acp **handshake 階段** fail-fast `Authentication required`，比 design 預期的 prompt-time surface 還更乾淨）
  - CX04 PASS（`AGENT_RUNTIME=codex` + `ACP_EXECUTABLE=claude-agent-acp` → spawn claude-agent-acp，stream MANGO）
- [x] 5.3 Regression sanity：MR01（claude default → APPLE）+ MR02（opencode → GRAPE）皆 PASS

## 6. 結束條件

- [x] 6.1 全套 QA cycle zero FAIL / zero env-fix-by-qa SKIP — `tests/test-report-2026-05-01-codex-support-1506.md`：CX01-04 + MR01-02 全 PASS（CX02、CX03 還比 design 預期更乾淨）
- [x] 6.2 README、Settings UI、code 三方一致：列項都是 claude / opencode / codex（README.md 含表格 + 「Codex 額外注意」段、SettingsPanel RadioGroup、`runtime.go::loadAgentRuntime` 三 case 對齊）
- [x] 6.3 Phase 0 pre-flight 結果已合併進 design.md（Q1/Q2 從 open question 變 verified；新增「Phase 0 pre-flight」段 + 「對 perch 的影響」表）
- [x] 6.4 archive 完成 — 2026-05-02 archived
