## Context

ACP `session/prompt` 在 perch 目前都送純 text block：

```go
// acp_process.go:215
p.call(ctx, "session/prompt", map[string]any{
    "sessionId": sessionID,
    "prompt":    []map[string]any{{"type": "text", "text": text}},
})
```

ACP / Anthropic 內容區塊格式（與本 change 設計依據）：

```jsonc
{"type": "text",  "text": "..."}
{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "<base64>"}}
{"type": "image", "source": {"type": "url",    "url": "https://..."}}
```

claude-agent-acp 2.x 對 image content 的支援需在實作 phase 0 verify（送一個含 image 的 prompt 並確認 RunCompleted 不報錯）。

Web Chat 前端目前只送 JSON `{"query": "...", "new_conversation": false, "conversation_id": "..."}`。Discord adapter `im_discord.go` 在 message handler 裡只取 `m.Content`，完全忽略 `m.Attachments`。

## Goals / Non-Goals

**Goals**
- Web Chat 與 Discord 兩個介面都能讓使用者把「圖片 + 文字」一次送給 Claude
- 對舊 client / 純文字 query 完全向下相容（無 attachments 就跟現在行為一樣）
- 大小、檔案數、MIME 白名單由 server 強制執行，避免 DoS / 惡意檔案
- Management history 列表不被 base64 撐爆（query 欄位顯示成 placeholder）

**Non-Goals**
- 不處理非圖片檔（PDF、文件、影片）— 另開 change
- 不做圖片預覽 / 縮圖 / 編輯 UI
- 不持久化 image（送完丟掉，不存 `/data` 也不存 workspace）
- 不處理 Telegram attachment（介面結構不同，待後續）
- 不做 ImageMagick 轉檔 / resize；前端送什麼進來，server 就轉什麼進 ACP

## Architecture

### Web Chat 流程

```
[browser] file picker / drag-drop
  ↓ FileReader.readAsDataURL → base64
[browser] POST /api/chat
  body: {
    query: "what's wrong here?",
    attachments: [
      {filename: "screenshot.png", mime_type: "image/png", data_base64: "iVBORw0KGgo..."},
    ],
    conversation_id: "..."
  }
  ↓
[chat_api_acp.go] StartSession()
  - 驗證 attachments[].mime_type ∈ allow list
  - 驗證 attachments[].size_bytes ≤ CHAT_UPLOAD_MAX_BYTES
  - 驗證 len(attachments) ≤ CHAT_UPLOAD_MAX_FILES
  - 組 ACP content blocks: [text(query), image(att1), image(att2), ...]
  ↓
[acp_process.go] PromptWithContent(ctx, blocks, callbacks)
  - session/prompt 帶 multi-block prompt
  ↓
ACP subprocess (claude-agent-acp)
  - 把 image content 餵給 Claude API（vision 模型）
  - 回傳 RunCompleted + accumulated text
```

### Discord 流程

```
[discord channel] 使用者 attach image + 打字
  ↓ Discord message event: m.Content="..." m.Attachments=[{URL, ContentType, Size, ...}]
[im_discord.go] message handler
  - 過濾 Attachments：ContentType ∈ allow list && Size ≤ MAX
  - HTTP GET each attachment URL → bytes
  - base64 encode
  - 組 ACP content blocks: [text(m.Content), image(att1), ...]
  ↓
[acp_session_pool] Acquire("discord:channel:<id>")
  ↓
[acp_process.go] PromptWithContent(ctx, blocks, ...)
```

### 共用資料結構（Go）

```go
type ACPContent struct {
    Type     string         `json:"type"`              // "text" | "image"
    Text     string         `json:"text,omitempty"`
    Source   *ACPImageSource `json:"source,omitempty"`
}

type ACPImageSource struct {
    Type      string `json:"type"`        // "base64"
    MediaType string `json:"media_type"`  // "image/png" 等
    Data      string `json:"data"`        // raw base64（無 data: prefix）
}
```

`PromptWithContent(ctx, blocks []ACPContent, onChunk, onToolStart, onToolEnd)` 為新主路徑；舊 `PromptWithChunks(text, ...)` 維持 thin wrapper：

```go
func (p *ACPProcess) PromptWithChunks(ctx, text, ...) {
    return p.PromptWithContent(ctx, []ACPContent{{Type:"text", Text:text}}, ...)
}
```

## Decisions

### D1：圖片只走 base64 inline，不走 URL

理由：
- 雖然 Discord attachment 有 CDN URL，但這 URL 帶 `?ex=...&hm=...` 簽章、~24 小時後失效，ACP subprocess 不能保證在期限內 fetch。
- Anthropic API 接受 base64，整體簡化（chat-API 與 Discord 兩條 path 用同一格式）。
- 限制：base64 比 raw bytes 大約 1.33 倍，10MB raw → 13MB JSON。可接受（chat-API 與 Discord HTTP body 都遠超此值）。

### D2：所有上傳路徑都在 server 端 re-validate

前端 + Discord platform 已有 enforcement，但 server **還是** 重新驗：
- MIME 白名單（讀第一 byte magic number 而非信任 client claim）
- 大小（Decoded base64 後再量一次）
- 檔案數
理由：純粹安全考量，client 端 enforcement 是 UX，不算 trust boundary。

### D3：上傳檔不持久化

不寫到 `/data`、不寫到 workspace、不存 query_log_store。理由：
- 隱私：截圖可能含 token、機敏資訊
- 大小：history 詳情頁不該變成 image gallery
- 簡單：lifecycle 跟著 ACP prompt 結束就丟

如果使用者要 reference 同張圖跨 session，自己再上傳一次（Anthropic API 也是 stateless 對待 image content）。

### D4：Management history 的 `query` 欄位顯示

`query_sessions.query` 欄位是 TEXT。三選一：
- (a) 存 placeholder：`[image:screenshot.png] [image:diagram.png] what's wrong?`
- (b) 存 raw query 不含 attachments，attachment 元資訊存到 tool_events 或新 column
- (c) 存 JSON：`{"text":"...","attachments":["screenshot.png"]}`

**選 (a)**：管理面板 / history list 一眼可讀；不需新 schema migration；列表不被 base64 撐爆。代價是 query 欄位混 markup，但對人讀可接受。

### D5：前端 attachments 入口

Chat UI textarea 旁加：
- file picker button（accept="image/*"）
- textarea 整塊 dropzone（drag-drop）
- pasted image（剪貼簿 paste event）
顯示已選檔案 chip + ✕ 刪除。送出後清空。

不做縮圖預覽（避免再加 image library）；只顯示檔名 + size。

### D6：環境變數可在 Settings UI 熱改

```
CHAT_UPLOAD_MAX_BYTES        預設 10485760 (10MB)
CHAT_UPLOAD_MAX_FILES        預設 4
CHAT_UPLOAD_ALLOWED_MIME     預設 "image/png,image/jpeg,image/gif,image/webp"
```

放進 `Settings → Chat`，比照既有 `RATE_LIMIT_RPM` pattern。生效時機：下次 query。

## Risks / Trade-offs

- **ACP subprocess 對大 base64 的吞吐**：13MB JSON over stdin，theoretical OK 但實測前不保證。Phase 0 量一次 P95 latency。
- **Chrome paste image** 的 MIME 是 `image/png` 沒檔名，UI 要 fabricate filename（`pasted-<timestamp>.png`）。
- **Discord attachment URL 失效**：fetch 失敗時要 graceful fallback（送出純文字 + 加 ❌ reaction 提示，而非整個 query 廢掉）。
- **ManagementHub `current_tool` 顯示**：image-only query Claude 可能不開 tool（純 vision 回答），對 live observability 沒影響但要 verify。
- **Claude Code 2.1.x ACP image 支援版本**：Phase 0 須測試現行 image content blocks 是否被接受；若 ACP 版本太舊要升 npm 套件。

## Migration Plan

無前置 schema migration、無需停機；前端 + 後端可獨立 deploy。完整路徑可用：

1. 先部 backend（接受 `attachments` 但前端還沒送）→ 全功能不可見、零回歸
2. 後部前端 → 自然啟用
3. Settings UI 設定可在前端發布前後任意時間設

## Open Questions

- ~~**Q1：是否允許在含 image 的 query 後啟用 tool use？**~~ **已決**：允許。`permissionMode: bypassPermissions` 與一般 query 行為一致；Phase 0 實測 ACP image+tool combination 是否有額外限制，沒有就保留現行行為。
- ~~**Q2：Discord attachment 包含 video / audio 時如何處理？**~~ **已決**：靜默 drop（不在 reply 提示）。理由：簡化使用體驗，不阻塞主要對話流。
- ~~**Q3：圖片要記入 query_log_store 的 tool_events 嗎？**~~ **已決**：不記。tool_events 是 ACP runtime 事件，圖片是 input 不是 tool 產出。後續若需 audit，另開 capability `attachment-log`。
- ~~**Q4：是否需要 Discord 反向（bot 回 Claude 生圖／回上傳檔）？**~~ **已決**：不在本 change 範圍。Claude Code 不會主動產生 image artifact；如有需求另開 change。
