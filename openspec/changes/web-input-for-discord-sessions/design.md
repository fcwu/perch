## Context

Perch 的 Web UI 可以透過 `/ws/session?id=<channelID>` WebSocket 訂閱 Discord PTY 的輸出串流。目前 `handleSessionWS` 在接收端只處理 resize 訊息，其他所有輸入（keystrokes）一律丟棄。`SessionProvider` 介面也沒有寫入方法，因此 Web 端完全無法向 PTY 注入輸入。

## Goals / Non-Goals

**Goals:**
- Web UI 使用者可在 `/ws/session` 對 Discord session 的 PTY 輸入文字（keystrokes）
- 介面最小擴充：僅在 `SessionProvider` 新增 `WriteSession`
- 向下相容：若 session 不支援寫入，伺服器靜默忽略

**Non-Goals:**
- 多人同時寫入的衝突控制（PTY 本身是 FIFO，先來先寫即可）
- Web UI 前端的 UI 調整（input 框等）留給前端自行決定
- Telegram 或其他 IM adapter 的 write 支援

## Decisions

**1. 在 `SessionProvider` 介面新增 `WriteSession`**

```go
WriteSession(channelID string, data []byte) error
```

理由：保持介面完整性，server 層不需要知道底層是 Discord 還是其他 adapter。替代方案（type assertion）會讓 server 與 Discord 耦合，捨棄。

**2. `handleSessionWS` 的輸入處理**

WebSocket 訊息分兩種：
- JSON `{"type":"resize","cols":N,"rows":N}` → resize
- 其他所有 binary/text → 視為 keystrokes，呼叫 `WriteSession`

先嘗試 JSON unmarshal，若成功且 type=resize → resize；否則直接寫入 PTY。

**3. `DiscordSessionManager.WriteSession` 實作**

找到 channelID 對應的 `*pty.PTY`，呼叫 `pty.Write(data)`。若找不到 session，回傳 error（server 層記 log 後忽略）。

## Risks / Trade-offs

- **多 Web 客戶端同時輸入** → PTY 本身是 FIFO 無鎖衝突，但使用者體驗可能混亂。暫不處理，屬已知限制。
- **`SessionProvider` 介面新增方法是 breaking change**（若有其他 adapter 實作）→ 目前只有 `DiscordSessionManager` 實作，風險低；若未來有其他 adapter，編譯器會強制補齊。

## Open Questions

無。
