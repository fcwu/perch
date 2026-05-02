## Context

`2026-05-01-chat-discord-image-upload`（已 archive）為 chat-API + Discord 加上了 image-only 附件路徑：前端把 file 讀成 base64，server 端驗證後用 `AttachmentsToACPBlocks` 組成 ACP `image` content block，跟 text 一起送進 `session/prompt`。整條路徑工作良好但被刻意限縮在 `image/png|jpeg|gif|webp` 四種 MIME。

實際使用後發現大量需求落在「非圖檔」：log 排錯、CSV 報表分析、PDF 規格書摘要、會議錄音轉錄、bug 重現錄影。多模態 LLM 對影片/音訊原生支援有限，PDF 與文字檔則完全不該擠進 context。但 perch 的 agent 都有 tool-use（Read/Bash），只要檔案落在它的 workdir 並告知路徑，agent 就能用既有工具自主分析。

本 change 的目標是讓 chat 與 Discord 的附件路徑變成「依 MIME 分流」：image 仍 inline、其他類型落盤 + path 引用，且第一階段先涵蓋 agent 已能用既有工具處理的格式（文字類 + PDF），影音與 runtime 工具升級延後到 Phase 2。

## Goals / Non-Goals

**Goals:**

- `/api/chat` 與 Discord adapter 接受混合 image / 非 image 附件，前端使用體驗一致
- 非 image 附件落到 `<workdir>/uploads/<conv-id>/<filename>`，agent 可用相對路徑（`./uploads/<conv-id>/<filename>`）讀取
- prompt 自動注入 `[file: ./uploads/<conv-id>/<filename> (mime, size)]` 前綴，agent 不需額外提示就知道有檔案可分析
- 每個 conversation 的 uploads 目錄獨立，會話結束（pool 退出 / history 刪除 / 啟動掃描遺留）即清理
- Wire shape 100% 不變（仍是 `{filename, mime_type, data_base64}`），server 端自行決定怎麼處理；舊 client 不需改動
- Image 路徑（ACP `image` block）零變動，行為與 throughput 不退步

**Non-Goals:**

- Phase 2 才處理 video/audio MIME 與 runtime image 的 ffprobe / whisper
- 不做檔案內容萃取（OCR、PDF 文字抽取、CSV schema 推論）—— 留給 agent 自行決定
- 不支援多人共用同一份附件（每個 conversation 自己一份；agent 也不能跨 conversation 看其他附件）
- 不支援大檔分塊上傳（單檔上限維持 server-side 一次 base64 接收；超過 50 MB 屬於不同問題）
- 不重新設計 history 顯示（沿用現有 `[image:foo.png]` 樣式，擴成 `[file:foo.csv]`）
- Telegram 非圖附件（與 Discord 訊息結構差異大，獨立 change）

## Decisions

### D1：非 image 走 disk-save，不走 ACP `image` / `audio` content block

**選擇**：非 image 附件由 server 寫入 `<workdir>/uploads/<conv-id>/<filename>`，並在 prompt 文字最前面加 `[file: ./uploads/<conv-id>/<filename> (mime, size)]`。Agent 用既有 Read/Bash 工具自主讀取。

**Why not ACP `image` block for non-image**：ACP `image` block 規範就是 image MIME，塞 PDF/log/audio 進去違規。

**Why not ACP `audio` content block (Phase 2 預留路徑)**：ACP 規格已有 `audio`，但 (a) 需要 base64 inline、context window 撐不住長音檔；(b) 不同 agent 後端對 audio 支援度不一（claude-code 有、opencode 不確定）；(c) 用 disk-save 配 whisper tool 反而通用。Phase 2 會回頭評估短音檔是否值得走 inline。

**Why not ACP `resource_link` 規格塊**：ACP schema 有定義 `resource_link` 但 perch 的 agent 們對此塊的支援度沒驗證過。`[file: ...]` 純文字前綴是「最低共通分母」—— 任何 agent 都能讀懂。日後若驗證 `resource_link` 通用，可平滑切換而不影響 wire shape。

**Why not 直接把檔案內容讀完塞進 prompt 文字**：
- 大檔（PDF、log）會吃光 context
- 二進位檔不能直接塞文字
- Agent 的 Read 工具有自己的 chunk / range 邏輯，比 server 提前讀更聰明

### D2：目錄結構 `<workdir>/uploads/<conv-id>/<filename>`

**選擇**：每個 conversation 一個子目錄，原檔名（經 sanitize）放在最底層。

**Filename sanitize 規則**：
- 移除路徑分隔符（`/`、`\`）與 `..`（防 path traversal）
- 限制 ASCII 可印字元 + 常見 unicode 字（中日韓檔名要保留）
- 重名加數字尾碼 `foo (2).pdf` —— 不覆寫，避免會話內第二次貼同名檔把第一次蓋掉
- 長度上限 200 char（含副檔名）

**Why per-conversation 子目錄**：
- Agent 看到 `./uploads/<conv-id>/...` 就知道哪個 conversation 的檔，跨會話污染不可能
- 清理單純：刪會話 = `rm -rf` 該目錄
- agent 偶爾會把工作檔產到 cwd，跟 uploads 目錄分開避免衝突

**Why not flat dir + uuid filename**：丟掉原檔名 = agent 看到 `7f3a...bin` 不知是什麼，使用者體驗也差。原檔名是元資訊。

### D3：Prompt 前綴格式 `[file: <relpath> (<mime>, <size>)]`

**選擇**：每個非 image 附件在 prompt 文字最前面各佔一行；多檔多行；接著一行空行才是使用者輸入。

範例：
```
[file: ./uploads/c-abc123/error.log (text/x-log, 142 KiB)]
[file: ./uploads/c-abc123/spec.pdf (application/pdf, 1.2 MiB)]

幫我看 log 對照 spec 找出哪段不符合規格
```

**Why 這個格式**：
- `[file: ...]` 與既有 `[image:foo.png]` history 樣式對齊
- 包 mime + size 讓 agent 一眼決定要不要先 `file` / `head` / `wc -l` 再 Read
- 相對路徑（`./uploads/...`）讓 agent 不用知道 absolute workdir
- 多檔分行避免 agent 把多份檔案搞混

**Why not JSON / YAML 結構**：agent 是 LLM，自然語言描述比結構化資料更容易被 reasoning 用上；`[file: ...]` 前綴又夠規律可被 server 端用 regex 從歷史訊息剝除（如果哪天要做）。

### D4：MIME 分流靠 `MagicMime` 擴充，不靠副檔名

**選擇**：保留現行「magic bytes 必須與 client 宣稱 MIME 一致」的嚴格驗證；`MagicMime` 擴充支援 PDF（`%PDF-`）、純文字（heuristic：UTF-8 valid + 無控制字元）。

**新增的 magic bytes**：
- PDF：`%PDF-` 前綴（4 byte）
- text/plain | text/markdown | text/csv | text/x-log | application/json | application/x-ndjson：用 UTF-8 valid + printable-ratio heuristic 判斷為「文字類」，再以 client 宣稱 MIME 為準（因為 csv/json/log/md 之間靠 magic bytes 無法區分）。Heuristic：前 8 KB 全為 valid UTF-8，且 control char (除 `\t \n \r`) 比例 < 1%。

**Why heuristic for text**：text/* 沒有 magic bytes 共識，只能驗「這份 byte 看起來不是二進位」。重點是擋掉「client 說我是 text/plain 但其實塞了 binary」的攻擊面，而不是區分 csv vs json（agent 能自己判斷）。

**Why not 信任副檔名**：副檔名來自 client，可任意偽造；server 端必須能獨立驗證 byte 真的是文字 / 真的是 PDF。

### D5：清理時機

**三條清理路徑**：

1. **Conversation pool 退出時**：`user_session.go` 的 ACP session pool 把 `(user_id, conv_id)` 從 pool 移除時（idle timeout / LRU evict / crash），刪除對應 `<workdir>/uploads/<conv-id>/`
2. **History 刪除時**：使用者透過 management UI 刪 conversation（如有此操作），同步刪 uploads 目錄
3. **啟動掃描**：perch 啟動時掃 `uploads/` 子目錄，凡目錄內最新檔 mtime 超過 `CHAT_UPLOAD_ORPHAN_TTL_DAYS`（預設 7 天）的全部刪掉。用 mtime 而非「conv-id 在 query_sessions 內」，因為 Discord channel 不寫進 `query_sessions`，但 mtime 自然反映「最近還在用」

**Why 不在每次 prompt 結束就刪**：使用者可能在多輪對話中要求 agent 反覆讀同一份檔；prompt 結束就刪 = agent 第二輪要 Read 時找不到檔。Conversation 級才是合理的 lifecycle。

**Why 啟動掃描而非 cron job**：perch 是 single-process service，有重啟機會就足以清孤兒；不引入 cron 複雜度。

### D6：每會話容量上限 `CHAT_UPLOAD_DIR_QUOTA_BYTES`

**選擇**：除既有 `CHAT_UPLOAD_MAX_BYTES`（單檔）+ `CHAT_UPLOAD_MAX_FILES`（單次 request 檔數）外，新增 `CHAT_UPLOAD_DIR_QUOTA_BYTES`（單會話累計，預設 500 MB）。Request 傳入時若 `du -sb <conv-dir> + new_size > quota` 則拒絕整個 request。

**Why 需要這個**：原本兩個 limit 只擋單次行為。多輪對話可能累積上百 MB；沒有 conversation-level cap 會讓壞行為（或 bug）把 disk 灌爆。

**Why 預設 500 MB**：典型容器 disk 數十 GB，500 MB / conv 撐 100 個 conv 也才 50 GB；對使用者單會話需求（幾份 PDF + 幾段 log）綽綽有餘。Admin 可調。

### D7：Image 路徑零變動

**選擇**：image MIME（PNG/JPEG/GIF/WebP）的 attachment 完全沿用現行 `AttachmentsToACPBlocks` → ACP `image` content block，不寫進 disk，不在 prompt 加 `[file: ...]` 前綴（多模態 LLM 直接看到 image，不需要文字提示）。

**Why**：
- 多模態 vision 是 image 的 superpower，落盤再叫 agent Read 等於降級
- 不變動 image 路徑 = 0 regression risk
- Server 端只多一個 `if isImageMIME(mime)` 分流

## Risks / Trade-offs

- **[Risk] Path traversal / filename injection**：使用者送來的 filename 可能含 `..` 或 NUL，落到 host 任意位置 → **Mitigation**：sanitize 嚴格白名單字元，拒絕 `..`、`/`、`\`、NUL；最終 path 用 `filepath.Clean` + 確認 prefix 是 `<workdir>/uploads/<conv-id>/`
- **[Risk] Disk 灌爆**：使用者連續上傳大檔 → **Mitigation**：D6 的三層 quota（單檔 / 單 request 檔數 / 單會話總量）
- **[Risk] 啟動掃描誤刪正在使用的目錄**：另一個 perch 實例同時在跑（不該發生但要防）→ **Mitigation**：啟動掃描只刪「conv-id 不存在於 query log store 最近 N 天」的目錄；單一 perch 部署假設沿用既有 single-instance 設計
- **[Risk] Agent 收到 `[file: ...]` 前綴但不理解 / 不去讀**：弱 agent 可能忽略提示直接回答 → **Mitigation**：Phase 1 只支援 claude-code / codex / opencode 這幾個有 Read 工具的 runtime；test plan 加上「agent 確實有讀檔」的 e2e 驗證
- **[Risk] Filename 重名衝突，第二次上傳同名檔覆寫第一次**：`error.log` 在會話多次出現 → **Mitigation**：D2 的「重名加 `(2)` 尾碼」策略；同會話內 unique 即可
- **[Trade-off] 落盤 = agent 多一次 Read 的延遲**：vs 直接 inline 進 prompt → 接受。Inline 不可行（context 撐不住 + binary 不能塞），且 Read 延遲 < 100ms 對使用者無感
- **[Trade-off] 不萃取 PDF 文字到 prompt**：每次對話都要 agent 自己 Read → 接受。Agent 自選 chunk / range 比 server 預先讀全文聰明，且減少 server 端依賴
- **[Trade-off] Discord 沒有 conv-id 概念，需 map 到 perch 的 conv key**：Discord 的 channel/thread 已 map 成 `discord:<channel>:<user>` 之類的 conv key（沿用現行 `discord-acp-session` 邏輯）；uploads 目錄就用同一個 key。沒有額外複雜度

## Migration Plan

1. **Phase 1（本 change）**：
   - 部署後舊 client 完全相容（只送 image 仍走原路徑）
   - 新前端 release 把 `accept` 放寬，使用者開始可以上傳非圖檔
   - 觀察 1-2 週 disk 使用量，視情況調 `CHAT_UPLOAD_DIR_QUOTA_BYTES` 預設值
2. **Rollback**：把 `CHAT_UPLOAD_ALLOWED_MIME` 環境變數重設成只含 image MIME 即可即時關閉非圖功能；前端會被 server 回 `mime_type not in allow-list` 擋下；無 schema 變更需 rollback
3. **Phase 2（另開 change）**：
   - 在 runtime image Dockerfile 加 ffprobe / whisper.cpp（或選擇 cloud transcription）
   - 把 `CHAT_UPLOAD_ALLOWED_MIME` 加入 video/audio MIME
   - 相同的 disk-save 路徑直接適用，不需改 server 邏輯

## Open Questions

- **Q1**：是否需要前端顯示「目前會話 uploads 已用 X / 500 MB」？—— 暫定不做，等使用者真的撞到 quota 再加
- **Q2**：是否要把 uploads 目錄掛 named volume（與 workspace 一致）？—— 暫定跟 workspace 同 volume，避免額外 mount 點。若日後 workspace 與 uploads 生命週期分離再切
- **Q3**：PDF 萃取是否要 server 預先呼叫 pdftotext 把純文字也存一份？—— 暫定不做，agent 自己 Read 即可；測完 Phase 1 再評估
