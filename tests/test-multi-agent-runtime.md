# Multi-agent ACP runtime 測試案例

> 功能：multi-agent-acp-runtime
> 涵蓋範圍：`AGENT_RUNTIME` 切換是否真的反映在 chat-API / Discord / Telegram 的 ACP subprocess（不再 hardcode `claude-agent-acp`）；env override precedence
> 撰寫日期：2026-05-01
> 相關 openspec：`agent-runtime-selection`、`acp-client`、`agent-runtime-integration`

---

## 共通前置

- 容器或本機 binary 跑 perch；同個 image 預裝 `claude-agent-acp`（npm）+ `opencode`（sst tarball）
- 切 runtime 必須**重啟** perch（`AGENT_RUNTIME` 在 startup 才讀）
- 測試圖片可重用 `tests/fixtures/tiny.png`

---

## E2E-curl

### MR01 — 預設 claude runtime（baseline，無回歸）

**層級**：E2E-curl

**Given** Perch 起時 `AGENT_RUNTIME` 未設（或設 `claude`）
**When**
```bash
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"reply with PINEAPPLE","new_conversation":true}'
sleep 3
ps aux | grep -E 'claude-agent-acp|opencode' | grep -v grep
```

**Then**
- 進程列表至少有一個 `claude-agent-acp` subprocess
- 沒有 `opencode acp` subprocess
- chat-API 收到正確回應（內含 `PINEAPPLE`）
- container log 出現 `ACP process started executable=claude-agent-acp`

---

### MR02 — `AGENT_RUNTIME=opencode` 切換後 chat-API 走 opencode

**層級**：E2E-curl

**前置操作**：把 perch 用 `AGENT_RUNTIME=opencode` 重啟。確認 image 內 `which opencode` + `opencode --version` 正常（pre-flight 已驗）。

**Given** Perch 起時 `AGENT_RUNTIME=opencode`
**When**
```bash
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"query":"reply with PINEAPPLE","new_conversation":true}'
sleep 5
ps aux | grep -E 'claude-agent-acp|opencode' | grep -v grep
```

**Then**
- 進程列表至少有一個 `opencode acp --log-level WARN` subprocess
- **不能**有 `claude-agent-acp` subprocess
- chat-API 收到正確回應（含 `PINEAPPLE`，模型會是 opencode default `opencode/big-pickle` 或設定的 model）
- container log 出現 `ACP process started executable=opencode`
- container log 出現一行 warning `acp: session/set_mode bypassPermissions failed (continuing)`（預期；opencode 不認此 mode，但 default `build` mode 仍能跑 tools）

**Cleanup（後置）**：把 `AGENT_RUNTIME` 改回 `claude` 並重啟，避免污染後續測試

---

### MR03 — `AGENT_RUNTIME=opencode` 下 image upload（CU01 重跑）

**層級**：E2E-curl

**Given** Perch `AGENT_RUNTIME=opencode` 跑著
**When**
```bash
B64=$(base64 -w0 < tests/fixtures/tiny.png)
curl -sS -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"describe color\",\"new_conversation\":true,\"attachments\":[{\"filename\":\"tiny.png\",\"mime_type\":\"image/png\",\"data_base64\":\"$B64\"}]}"
```

**Then**
- HTTP 200，回應含 `black`/`dark` 之類描述（OpenCode 也得有 vision 支援，由 model 決定）
- 若 selected model 不支援 vision（例如 `opencode/nemotron-3-super-free` 純文字模型），這個 case 標 SKIP-as-needs-user-action 並註明「OpenCode default model lacks vision」

> 註：OpenCode `promptCapabilities.image: true` 是 ACP 層級宣告，但實際 model 是否吃 image 由 selected model 決定（OpenCode UI 可切 model）。perch 不負責驗 model-level vision；只驗 perch → opencode 的 protocol path 不報錯。

**Cleanup**：同 MR02

---

### MR04 — `ACP_EXECUTABLE` env override 優先於 runtime

**層級**：E2E-curl

**Given** Perch 起時同設 `AGENT_RUNTIME=opencode` + `ACP_EXECUTABLE=claude-agent-acp`
**When** 同 MR02 的 curl
**Then**
- 進程列表是 `claude-agent-acp` subprocess（env override 生效）
- **不**起 opencode subprocess
- container log 出現 `ACP process started executable=claude-agent-acp`

**反向驗證**：同時設 `ACP_EXECUTABLE_ARGS='["--debug"]'` → log 會顯示 args 被覆蓋；invalid JSON 應 warning 並 fallback

**Cleanup**：移除兩個 env、重啟

---

## Sanity（驗 claude path 無 regression）

跑既有 baseline subset：MT12 / T55-multi / T56 / T19 / CU01。預期全 PASS（同 archive 後的 `tests/test-report-2026-05-01-image-upload-round2-1022.md`）。

---

## 備註

- MR03 OpenCode vision 結果視 selected model 而異，**功能不通過 ≠ perch bug**（OpenCode model 層問題）
- MR04 也是 D6 precedence 的唯一驗證點
- 若 MR02 失敗（opencode subprocess 起不起來、`opencode auth login` 缺、stdout polluted by INFO log），優先檢查：
  - `docker exec ... which opencode` 是否存在
  - `docker exec ... opencode --version` 是否回 `1.14.x` 或更新
  - `ACPArgs=["acp","--log-level","WARN"]` 是否真的傳給 subprocess（log 有 ACP process started 且 sessionId 非空 = 路徑通）
  - 是否需要 `opencode auth login`（free models 不需，paid models 需）
