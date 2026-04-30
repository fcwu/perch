# Telegram ACP 測試案例

> 功能：telegram-acp-migration
> 涵蓋範圍：TelegramAdapter ACP mode — per-chat session pool、多輪對話、crash 重啟、idle timeout。
> 撰寫日期：2026-04-30

---

## E2E-manual — Telegram ACP

### TG-A01 — Telegram 個人 chat → ACP run → 回應正確

**層級**：E2E-manual（需 Telegram bot token + 允許的 chatID）

**Given** 容器以正確 `TELEGRAM_TOKEN` 與 `TELEGRAM_CHAT_ID` 啟動，`/etc/perch-claude-host` 已掛載
**When** 在 Telegram 個人 chat 傳訊息 `echo hello`
**Then**
- bot 回應包含 `hello`
- container log 出現 `ACP pool: session created key=telegram:chat:<chatID>`

**驗證方式（手動）：**
```
Telegram → bot 個人 chat → 輸入：echo hello
預期 bot 回應：包含 "hello"
```

---

### TG-A02 — Telegram group chat（@提及 bot）→ ACP run

**層級**：E2E-manual

**Given** bot 加入 group，且 `TELEGRAM_CHAT_ID` 設為 group 的 chatID
**When** 在 group 中傳任何訊息（Telegram group 不過濾 @mention，所有訊息均觸發）
**Then** bot 回應訊息

> 注意：目前 TelegramAdapter 以 chatID 全匹配過濾，group 需設定對應 group ID。

---

### TG-A03 — 多輪對話 context 保留

**層級**：E2E-manual

**Given** 同一個 chat 已和 bot 對話一輪（第一輪問 "My name is Alice"）
**When** 第二輪問 "What's my name?"
**Then** bot 回應包含 "Alice"（ACP session 持久化保留 context）

**驗證：**
```
Telegram → "My name is Alice"
等待回應後 → "What's my name?"
預期回應包含 "Alice"
```

---

### TG-A04 — subprocess crash → 下次訊息自動重啟

**層級**：E2E-curl（需搭配容器內手動 kill）

**Given** container 正在跑，已有 Telegram ACP session
**When** `docker exec perch-local-test pkill -f claude-agent-acp`（殺掉 ACP subprocess）後再送一則訊息
**Then**
- bot 仍然回應（pool 偵測 crash，下次 Acquire 重啟新 subprocess）
- container log 出現 `ACP pool: session created`（新建 session）

**驗證指令：**
```bash
# 殺掉 ACP subprocess
docker exec perch-local-test pkill -f claude-agent-acp || true
# 然後在 Telegram 傳 "ping"，預期 bot 正常回應
docker logs perch-local-test 2>&1 | grep "ACP pool: session created"
```

---

### TG-A05 — idle timeout → subprocess 釋放，下次訊息重啟

**層級**：unit（可在 acp_session_pool_test.go 驗證）

**Given** Telegram ACP pool 設定 idle timeout = 50ms（僅 unit test 環境）
**When** Release 後等待 200ms
**Then**
- pool 中 session 消失
- 下次 Acquire 自動建新 session

**驗證：** 參見 `acp_session_pool_test.go` 中 `TestACPSessionPool_IdleTimeout`（已通過）
