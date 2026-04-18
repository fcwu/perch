## Context

Perch 的核心能力其實已經分成兩層：上層是 PTY、WebSocket、scheduler、Discord session、hook endpoint；下層是「啟動哪個 agent CLI」與「如何把該 agent 需要的設定檔放到正確位置」。目前下層完全寫死在 Claude Code：`main.go` 和 `im_discord.go` 直接呼叫 `claude`，`entrypoint.sh` 只會複製 `claude/skills` 並 merge `claude/settings.json`，runtime image 也只安裝 `@anthropic-ai/claude-code`。

這讓 Discord、schedule、hook callback 雖然已經和 PTY 架構整合，但只能服務 Claude。repo 內已有 `.opencode/` plugin 目錄，代表 OpenCode 已經被部分準備，但還缺少正式 runtime 選擇、安裝、設定注入與行為驗證，因此目前無法作為 Perch 的一級 runtime 使用。

## Goals / Non-Goals

**Goals:**
- 讓 Perch 可用單一設定選擇 `claude` 或 `opencode` 作為 agent runtime
- 主 PTY、Discord 專屬 PTY、scheduler 目標 PTY 都透過同一套 runtime abstraction 啟動，避免分支邏輯散落
- 讓 runtime-specific project config 注入機制可同時支援 Claude 與 OpenCode，至少覆蓋 hooks、skills/plugins 與工作目錄內設定資產
- 保持既有 Claude 預設行為不變；未設定 runtime 時仍以 Claude 為預設
- 將 OpenCode parity 要求落成可驗證需求，特別是 Discord、schedule 與 hook-driven 通知鏈路

**Non-Goals:**
- 不在這個 change 中重新設計 PTY、Discord session、scheduler 或 WebSocket 架構
- 不要求 Claude 與 OpenCode 共用完全相同的設定檔格式；只要求 Perch 對使用者暴露等價能力
- 不支援同一個 Perch instance 同時跑多種 runtime；本次只做單一 active runtime
- 不處理 OpenCode 本身不支援的功能模擬，除非能以 Perch 端明確補足且不破壞現有流程

## Decisions

### D1: 新增 `AgentRuntime` 抽象，統一描述 CLI、args、env 與設定目錄

Perch 應新增一個 runtime descriptor，例如 `AgentRuntime`，負責回答以下問題：
- 主命令是什麼（`claude` / `opencode`）
- 預設 args 與使用者額外 args 如何組合
- 需要的 home/workspace 子目錄名稱是什麼（例如 `.claude`、`.opencode`）
- 是否支援 hook 設定注入、對應設定檔路徑為何
- 要複製哪些 bundled assets（skills、plugins、merge script、settings template）

`main.go`、`im_discord.go`、可能的其他 session 啟動點都不再直接拼 `claude` 命令，而是向 runtime 取 `Command`, `ArgsForSession(target)`, `ExtraEnv(target)`。

為什麼不用只加一個 `if runtime == "opencode"`：目前啟動點已經分散在 main PTY、Discord PTY、entrypoint 三處，單純加條件分支會很快失控，也很難保證所有入口維持一致。

### D2: 用單一環境變數選擇 runtime，維持 Claude 為預設

新增一個 runtime 選擇設定，例如 `AGENT_RUNTIME`（值為 `claude` 或 `opencode`）。若未設定，仍視為 `claude`，避免破壞現有部署、README 範例與既有使用者習慣。

保留 runtime-specific 額外參數環境變數是合理的，例如沿用 `CLAUDE_ARGS` 給 Claude，並新增 `OPENCODE_ARGS` 給 OpenCode；但 PTY 啟動點只應面對 runtime abstraction，不直接知道這些變數名稱。

為什麼不用重載 `CLAUDE_ARGS` 讓它同時服務 OpenCode：名稱會誤導，文件也會更難理解。runtime-specific args 應與 runtime 選擇分開。

### D3: Entry point 改成 runtime-aware asset sync，而不是只處理 `.claude`

`entrypoint.sh` 現在有兩個重要職責：
- 複製 bundled skills 到 workspace 的 project-level 設定目錄
- 在 IM integration 開啟時，merge hooks 到 project-level settings.json

這兩件事都應改為 runtime-aware：
- Claude runtime 繼續寫入 `$WORKDIR/.claude/...`
- OpenCode runtime 改寫入其對應的 project config 目錄與 plugin 位置

實作上可把 `claude/` 擴充成通用 `runtime-assets/<runtime>/...`，也可保留 `claude/` 並新增 `opencode/` 目錄，只要 entrypoint 透過 runtime 名稱決定來源與目標。重點是 sync/merge 規則由 runtime descriptor 驅動，而不是寫死 `.claude`。

為什麼不直接沿用 `.claude` 到 OpenCode：這會把 Claude 的設定模型偷偷耦合到另一個 runtime，日後 OpenCode 若需要不同格式或檔名會更難調整。

### D4: Hook parity 以「runtime-native callback first, Perch fallback second」處理

Discord 目前依賴 hook callback 來接收 `PreToolUse` / `PostToolUse` / `Stop`。對 OpenCode，需求是使用者看起來仍有這條通知鏈路，但不應先假設 OpenCode 的 hook 格式與 Claude 完全一致。

因此設計上要求 runtime descriptor 聲明其中一種能力：
- 有原生 hook / event callback，可直接注入到 `/hook` 或新的 runtime endpoint
- 沒有相同能力，則必須明確定義 Perch fallback 行為，至少能在 completion 時把結果送回 Discord / schedule 目標

本 change 的 spec 會要求 OpenCode runtime 達成既有使用者可感知的 parity，但 design 保留具體注入格式由實作階段依 OpenCode CLI 能力驗證。

### D5: Discord session naming / target routing 維持不變，僅替換底層 runtime

目前 `discord:<channelID>` target 字串已用於 scheduler、PTY 命名與 webhook routing。這層抽象已經足夠，不需要為 OpenCode 再新增另一種 target 格式。OpenCode 支援應只改變該 target 對應的 agent process，而不改變 session target contract。

為什麼不把 runtime 放進 target，如 `opencode:discord:<id>`：這會把「部署層設定」混進「session routing」，增加 scheduler 與 session provider 複雜度，而 Perch instance 本來就只會有一個 active runtime。

## Risks / Trade-offs

- [OpenCode hook 能力與 Claude 不同] → 先以 spec 固定使用者可見行為，再在實作階段驗證 OpenCode 原生能力；若能力不足，明確定義最小 fallback，而不是偷偷降級
- [entrypoint sync 邏輯變複雜] → 用 runtime descriptor 提供來源/目標路徑，避免 shell 腳本硬編碼大量條件分支
- [新增 runtime 後測試矩陣擴大] → 以 runtime-neutral 測試覆蓋共同行為，再補少量 runtime-specific smoke tests
- [README 與部署文件容易混亂] → 將文件改成「先選 runtime，再看對應掛載／args」，而不是在每個功能章節重複描述

## Migration Plan

1. 新版 image 內同時具備 Claude 與 OpenCode runtime 支援，但預設仍使用 `claude`
2. 舊部署不需修改環境變數即可維持原行為
3. 要切換到 OpenCode 的部署，新增 runtime 選擇變數與 OpenCode 所需的掛載／token／args
4. 若 OpenCode parity 驗證不符預期，可將 runtime 設回 `claude` 回退，不需回滾整個 Perch 架構

## Open Questions

- OpenCode CLI 的正式命令名稱、非互動模式旗標、以及 session naming 能力是否可完全比照目前 `claude --name <target>` 流程
- OpenCode 是否有原生 hook / event callback 設定；若有，設定格式與 project-level config 路徑為何
- `.opencode/` 目前 repo 內 plugin 目錄在正式 runtime 下應掛到 workspace config、HOME config，或兩者都需要
