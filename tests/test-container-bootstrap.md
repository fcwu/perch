# Container Bootstrap 測試案例

> 功能：claude-container-bootstrap
> 涵蓋範圍：entrypoint.sh 的 cp /etc/perch-claude-host、.claude.json onboarding seed、fresh container 行為。
> 撰寫日期：2026-04-30

---

## E2E-curl — Container Bootstrap

### TBC01 — Fresh container（host ~/.claude:ro 直接 mount）能正常起動，Discord PTY 第一句有 reaction

**層級**：E2E-curl

**Given** docker-compose 掛載 `${HOME}/.claude:/etc/perch-claude-host:ro`，無 `tests/test-perchuser/.claude.json` 預先設定
**When** container 啟動後，Discord channel 送第一句訊息
**Then**
- container 啟動 log 出現 `perch listening`
- Discord 訊息在 5 秒內出現 👀 reaction（hooks 有被載入）
- `docker exec` 進容器確認 `/home/perchuser/.claude/` 為可寫目錄（非 RO mount）

**驗證指令：**
```bash
# 確認 .claude 是容器本地可寫副本，而非 RO bind mount
docker exec perch-local-test sh -c 'touch /home/perchuser/.claude/.test_write && rm /home/perchuser/.claude/.test_write && echo "writable"'
# 確認 onboarding flags 已 seed
docker exec perch-local-test cat /home/perchuser/.claude.json | python3 -m json.tool
```

---

### TBC02 — 已存在 `hasCompletedOnboarding=true` 時不被覆寫

**層級**：E2E-curl

**Given** host `~/.claude.json` 已有 `hasCompletedOnboarding=true`，entrypoint cp 後會把此值帶進容器
**When** container 啟動，entrypoint.sh 執行 jq seed
**Then** `/home/perchuser/.claude.json` 的 `hasCompletedOnboarding` 仍為 `true`（不被重寫）

**驗證指令：**
```bash
docker exec perch-local-test jq '.hasCompletedOnboarding' /home/perchuser/.claude.json
# 期望輸出：true
```

---

### TBC03 — cp 失敗時 entrypoint log warning 但仍啟動

**層級**：E2E-curl

**Given** `/etc/perch-claude-host` 不存在（未掛載 staging mount）
**When** container 啟動
**Then**
- container 正常起動，log 出現 `perch listening`
- `/home/perchuser/.claude/` 為空目錄（mkdir -p 建立）
- `/home/perchuser/.claude.json` 存在且有 onboarding flags seed

**驗證指令：**
```bash
docker logs perch-local-test 2>&1 | grep "perch entrypoint"
docker exec perch-local-test ls /home/perchuser/.claude/
docker exec perch-local-test cat /home/perchuser/.claude.json
```

---

### TBC04 — Claude Code Bash 工具能成功執行

**層級**：E2E-curl

**Given** container 使用新 mount 慣例（`/etc/perch-claude-host:ro`）啟動
**When** 透過 Chat API 送一個使用 Bash tool 的請求（例如 `echo hello`）
**Then**
- Chat API response 包含 Bash tool 的執行結果
- `session-env/` 目錄可在容器 `/home/perchuser/.claude/` 下寫入（不再 EROFS）

**驗證指令（搭配 tests/test-kb-chat-api.md 的 T55 / T56 流程）：**
```bash
# 送一個會觸發 Bash tool 的 chat request，確認 response 成功回傳
curl -s -X POST http://localhost:8082/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "run: echo hello from container"}' | jq '.response'
```
