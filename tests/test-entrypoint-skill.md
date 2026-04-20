# Entrypoint 與 Skill 合併 測試案例

> 功能：entrypoint-skill
> 涵蓋範圍：內建 skill 自動複製到 workspace、不覆寫主機 settings.json。
> 撰寫日期：2026-04-20

---

## T26 — Entrypoint Skill 合併

**層級**：E2E-browser

**Given** 使用者掛載了自己的 `~/.claude`（其中不含 `local-schedule` skill），並啟動 Perch container
**When** 使用者在 terminal 中告訴 Claude 設定一個排程
**Then**
- Claude 能夠使用 `local-schedule` skill 完成排程設定，不出現「skill 不存在」的錯誤
- 主機原有的 `~/.claude/settings.json` 內容未被修改（可在 container 外確認）

**反向驗證（IM 未設定時）**：
- 不設定 Discord 或 Telegram 環境變數的情況下啟動
- workspace 中不應出現 `.claude/settings.json`（hooks 不應被寫入）
