# OpenCode Runtime 測試案例

> 功能：opencode-runtime
> 涵蓋範圍：OpenCode 啟動、Discord 整合、排程觸發、Web UI 輸入。
> 相關 openspec：`add-opencode-support`。
> 撰寫日期：2026-04-20

---

## T40 — OpenCode Runtime 可啟動

**層級**：E2E-browser

**Given** Perch 以 `AGENT_RUNTIME=opencode` 及有效的 `ANTHROPIC_API_KEY` 啟動
**When** 使用者在瀏覽器開啟 terminal
**Then**
- terminal 顯示 OpenCode 的啟動畫面，而非 Claude Code 的畫面
- 頁面可正常使用，不出現「找不到 claude」或「無效 runtime」的錯誤

---

## T41 — OpenCode Runtime：Discord 訊息可收到完成回覆

**層級**：E2E-browser（含 Discord 整合）

**Given** Perch 以 `AGENT_RUNTIME=opencode` 及 Discord bot 設定啟動
**When** 使用者在 Discord channel 傳送簡短訊息，例如「回我一個 hi」
**Then**
- 原始訊息出現 👀 reaction，表示已被接收
- OpenCode 處理完成後，Bot 在同一 channel 以 reply 回覆結果

---

## T42 — OpenCode Runtime：Discord 排程仍回到正確 Channel

**層級**：E2E-browser（含 Discord 整合）

**Given** Perch 以 `AGENT_RUNTIME=opencode` 啟動，並設有 Discord bot
**When** 排程觸發，目標為特定 Discord channel
**Then**
- Discord channel 先出現 `📅 local schedule > ...` 的 header 訊息
- OpenCode 完成後，回覆出現在同一 channel，緊接在 header 下方
- 主 terminal 沒有收到這次 Discord 排程的任何輸出

---

## T43 — Web UI 對 Discord Session PTY 輸入

**層級**：E2E-browser

**Given** Discord bot 已啟動，Web UI 上可見 Discord channel tab
**When** 使用者在瀏覽器切換到 Discord channel tab，並在 terminal 中輸入文字（例如 `ls`）並按 Enter
**Then**
- 輸入的文字出現在 terminal 畫面
- PTY 執行該指令並顯示輸出
- 調整視窗大小時，terminal 自動重新填滿，不影響輸入行為
