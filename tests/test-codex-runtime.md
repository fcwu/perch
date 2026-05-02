# Codex ACP runtime 測試案例

> 功能：add-codex-support
> 涵蓋範圍：`AGENT_RUNTIME=codex` 是否真的把 chat-API / Discord / Telegram 切到 `codex-acp` subprocess；env override precedence 對 codex 生效；codex 預設 `read-only` mode 的影響
> 撰寫日期：2026-05-01
> 相關 openspec：`add-codex-support`（specs/agent-runtime-selection、agent-runtime-integration、acp-client）

---

## 共通前置

- 容器 image 同時預裝 `claude-agent-acp`（npm）+ `opencode`（sst tarball）+ `codex-acp`（npm `@zed-industries/codex-acp` + 對應 arch 的 platform package）
- 切 runtime **必須重啟** perch（`AGENT_RUNTIME` 在 startup 才讀）
- 跑 codex case 前先確認 `OPENAI_API_KEY` 已注入（`-e OPENAI_API_KEY=sk-...`）；無 key 時跑 prompt 會 surface 401，但 handshake 仍能起（CX03 專測這個）
- 測試圖片可重用 `tests/fixtures/tiny.png`

---

## E2E-curl

### CX01 — `AGENT_RUNTIME=codex` baseline 啟動成功 + handshake OK

**層級**：E2E-curl

**前置操作**：把 perch 用 `AGENT_RUNTIME=codex` + `OPENAI_API_KEY=sk-...` 重啟。確認 image 內 `which codex-acp` + `codex-acp --help` 正常。

**Given** Perch 起時 `AGENT_RUNTIME=codex`
**When**
```bash
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"reply with PINEAPPLE","new_conversation":true}'
sleep 5
docker exec <container> ps aux | grep -E 'claude-agent-acp|opencode|codex-acp' | grep -v grep
```

**Then**
- 進程列表至少有一個 `codex-acp` subprocess
- **不**有 `claude-agent-acp` subprocess、**不**有 `opencode acp` subprocess
- chat-API 收到正確回應（內含 `PINEAPPLE`；codex 預設 `read-only` mode 不影響純文字回答）
- container log 出現 `ACP process started executable=codex-acp`
- container log 出現一行 warning `acp: session/set_mode bypassPermissions failed (continuing)`（預期；codex 不認此 modeId，graceful warning-and-continue 既有路徑）

**Cleanup**：把 `AGENT_RUNTIME` 改回 `claude` 並重啟，避免污染後續測試

---

### CX02 — Codex `read-only` mode 限制（已知行為，非 bug）

**層級**：E2E-curl

**Given** Perch `AGENT_RUNTIME=codex` 跑著
**When** 送一個 prompt 要求 codex 編輯檔案，例如：
```bash
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"create a file /tmp/perch-codex-test.txt with content HELLO","new_conversation":true}'
```

**Then**
- HTTP 200；codex 回應**會**告知無法直接編輯（或回 read-only / approval-required 訊息）— 預期行為
- `/tmp/perch-codex-test.txt` 在容器內**不會**被建立
- 這個 case 證實「codex 預設 `read-only` mode」設計，不算 FAIL；標 NEEDS-USER-ACTION：「future change 加 runtime-aware mode mapping 才能改檔案」

**Cleanup**：同 CX01

---

### CX03 — `OPENAI_API_KEY` 缺失時 prompt 階段 surface error

**層級**：E2E-curl

**前置操作**：起 perch 時**不**設 `OPENAI_API_KEY`，`AGENT_RUNTIME=codex`

**Given** Perch `AGENT_RUNTIME=codex` 但無 OPENAI_API_KEY
**When**
```bash
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"hello","new_conversation":true}'
```

**Then**
- handshake 階段**不**fail-fast（codex-acp 容許 unauthenticated 起 session；pre-flight 已驗）
- 但送 prompt 後 codex-acp 收到 OpenAI 401 → ACP error → perch propagate 到 chat UI
- chat-API HTTP 響應為 error 訊息（含 `401`、`api key`、`unauthorized` 等關鍵字）
- container log 在 codex-acp stderr 段看到 `failed to connect to websocket: HTTP error: 401 Unauthorized`

**Cleanup**：補回 `OPENAI_API_KEY` 並重啟

---

### CX04 — `ACP_EXECUTABLE` env override 對 codex 生效

**層級**：E2E-curl

**Given** Perch 起時同設 `AGENT_RUNTIME=codex` + `ACP_EXECUTABLE=claude-agent-acp`
**When** 同 CX01 的 curl
**Then**
- 進程列表是 `claude-agent-acp` subprocess（env override 生效，與 MR04 一致）
- **不**起 codex-acp subprocess
- container log 出現 `ACP process started executable=claude-agent-acp`

**反向驗證**：把 `ACP_EXECUTABLE` 撤掉、加 `ACP_EXECUTABLE_ARGS='["-c","model=\"o3\""]'` → 進程是 `codex-acp -c model="o3"`（codex-acp 唯一支援的 flag 是 `-c key=value`）

**Cleanup**：移除兩個 env、重啟

---

## Sanity（驗 claude / opencode path 無 regression）

跑既有 baseline subset：

- **MR01**（claude default）→ 預期 PASS
- **MR02**（opencode subprocess 切換）→ 預期 PASS
- **MT12 / T55-multi / T56**（chat / Discord / Telegram baseline）→ 預期 PASS（與 multi-agent-acp-runtime archive 後的 `tests/test-report-2026-05-01-multi-agent-runtime-1318.md` 對齊）

---

## 備註

- **CX01 不需 vision**：純文字 prompt 即可；image upload (analog of MR03) 留給後續驗（codex `promptCapabilities.image: true` 已宣告，但 model 端是否真吃 image 由 OpenAI model 決定，本 change 不負責 model-level vision）
- **CX02 是「設計驗證」非 PASS/FAIL**：confirms 預設 read-only behavior，記錄 future change 觸發點
- **CX04 反向驗證**：`-c key=value` 是 codex-acp 唯一接受的 CLI flag，`--debug` 之類旗標不存在；測 args override 用 `-c model=...` 最穩
- 若 CX01 失敗：
  - 檢查 `docker exec ... which codex-acp` 是否存在（`/usr/local/bin/codex-acp` 應該是 npm shim）
  - 檢查 platform package（`@zed-industries/codex-acp-linux-x64` 或 `linux-arm64`）是否確實裝起來；無則 codex-acp shim 會印 `Failed to locate ... binary`
  - 檢查 `OPENAI_API_KEY` 是否傳到 subprocess（`acp_process.go::buildEnv` 應該 inherit container env）
  - 檢查 startup log 是否有 `runtime=codex`（若印 `runtime=claude` 表示 env 沒讀進來）
