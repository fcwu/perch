## Context

Perch 以單一 PTY session 執行 Claude Code，所有輸入都是「往 PTY 寫文字」，輸出透過 framebuffer broadcast 給 WebSocket 訂閱者。

現在要新增第三個輸入來源（繼 WebSocket、Scheduler 之後）：IM Bot。同時需要一個「輸出回路」——Claude 執行完畢後，把結果送回 IM，而不只是顯示在瀏覽器。

輸出回路的觸發點是 Claude Code Hooks：Claude 在特定事件（PreToolUse / PostToolUse / Stop）呼叫外部程式，傳入 JSON。

## Goals / Non-Goals

**Goals:**
- Discord bot：接收指定 channel 的訊息 → 寫入 PTY；Hook 事件 → emoji reaction + 文字回應
- Telegram bot：接收指定 chat 的訊息 → 寫入 PTY；Stop 事件 → 文字回應
- 兩個 adapter 都是 optional，由環境變數決定是否啟動
- Hook 設定 bake 進 image，容器啟動即生效

**Non-Goals:**
- 多人同時對話的並發隔離（簡單版：last-message-ID queue）
- Discord slash commands
- 訊息長度超過 2000 字的分頁處理（初版截斷或附檔）
- Webhook mode（使用 polling）

## Decisions

### D1：IM adapter 用 interface 抽象

```
type IMAdapter interface {
    Start() error
    Stop()
    Notify(event HookEvent, text string) error
}
```

`IMManager` 持有 `[]IMAdapter`，統一 Start/Stop，Hook 事件廣播給所有 adapter。

**為什麼**：Discord 和 Telegram API 差異大（reaction vs 純文字），但 Perch 核心不應感知差異。未來加 Line、Slack 只要新增 adapter。

### D2：Hook 用 HTTP endpoint，不用 stdin pipe

Claude Code Hook 設定：
```json
{
  "hooks": {
    "PreToolUse":  [{"command": "curl -sf -X POST http://localhost${LISTEN_ADDR}/hook -H 'Content-Type: application/json' -d @-"}],
    "PostToolUse": [{"command": "curl -sf -X POST http://localhost${LISTEN_ADDR}/hook -H 'Content-Type: application/json' -d @-"}],
    "Stop":        [{"command": "curl -sf -X POST http://localhost${LISTEN_ADDR}/hook -H 'Content-Type: application/json' -d @-"}]
  }
}
```

**為什麼**：Perch 已有 HTTP server，不需要額外的 IPC 機制。Hook 的 JSON payload 直接 pipe 進 curl，Perch 解析後廣播給 IMManager。

**為什麼不用 Unix socket**：Docker 環境下路徑管理複雜，HTTP localhost 更簡單。

### D3：IM 訊息與 Hook 事件的關聯用 last-message queue

```
IMManager 持有:
  lastMsg map[string]PendingMessage  // platform → {messageID, channelID, timestamp}
```

收到訊息 → 記錄 lastMsg → 寫 PTY。
Hook 事件 → 讀 lastMsg → 對該訊息加 reaction / 回覆。

**為什麼**：使用情境是個人使用，同一時間只有一個對話。用完整 correlation ID 需要修改 Claude 的輸入格式，複雜度不值得。

**Trade-off**：如果瀏覽器和 Discord 同時輸入，Hook 只會回應最後一則 IM 訊息。這是已知限制，文件說明即可。

### D4：Discord reaction emoji 狀態機

```
收到訊息 → 加 👀
PreToolUse  → 加 ⚙️
PostToolUse 成功 → 加 ✅，移除 ⚙️
PostToolUse 失敗 → 加 ❌，移除 ⚙️
Stop        → 加 💬，移除 👀 / ⚙️，送文字訊息
```

reaction 加在原始訊息上，不新增訊息。文字回應在 Stop 時以 reply 方式送出。

### D5：settings.json bake 進 image，不掛 volume 覆蓋

`claude/settings.json` 在 build time COPY 到 `/root/.claude/settings.json`。

**為什麼**：用戶掛載 `-v ~/.claude:/root/.claude` 會覆蓋整個目錄，包括 settings.json。改成只掛載 credentials：

```
-v ~/.claude/.credentials.json:/root/.claude/.credentials.json
```

或者在 entrypoint script merge settings，但這樣複雜度更高。初版文件說明這個限制。

## Risks / Trade-offs

- **Discord rate limit**：短時間大量 reaction 呼叫可能被 rate limit → 加 reaction 失敗時 log warning，不 crash
- **Hook curl 失敗**：如果 Perch 還沒啟動就觸發 hook，curl 失敗 → `-sf` flag 靜默失敗，不影響 Claude 執行
- **settings.json 被 volume 覆蓋**：見 D5，文件說明
- **PTY 輸出解析**：Stop hook 的 `result` 欄位只有 exit status，沒有 Claude 的完整回應文字。完整文字需要從 PTY framebuffer 截取或讓 Claude 用工具輸出。初版用 Stop hook 的 `transcript` 欄位（若有）或提示用戶在回應末尾加固定標記。

## Open Questions

- Stop hook 的 JSON payload 有沒有 Claude 的完整回應文字？需要實測確認。
- 如果沒有，初版回應策略：送固定文字「Claude 已完成，請查看瀏覽器」還是嘗試截取 PTY framebuffer 最後 N 行？
