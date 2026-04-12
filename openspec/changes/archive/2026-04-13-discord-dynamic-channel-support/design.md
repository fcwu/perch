## Context

Perch 的 Discord 整合目前使用 `DISCORD_CHANNEL_ID` + `DISCORD_BOT_TOKEN` 雙環境變數來啟動 Bot。`main.go` 的初始化條件要求兩者同時存在，否則不建立 `DiscordSessionManager`。`im_discord.go` 的 `onMessage` 已有「若 `allowedChannelID` 為空則不過濾」的邏輯，但從未被觸發，因為空字串的條件讓 Bot 根本不會啟動。

目標是讓使用者只需設定 `DISCORD_BOT_TOKEN`，Bot 即可被邀請到任意 Server，於所有頻道運作。

## Goals / Non-Goals

**Goals:**
- `DISCORD_CHANNEL_ID` 改為選填，不設定時 Bot 監聽所有頻道
- DM 直接對話，不需 @mention
- Private Guild 頻道（非 @everyone 可見）直接回應，不需 @mention
- Public Guild 頻道需 @mention 觸發（防止 Bot 干擾所有對話）
- 設定 `DISCORD_CHANNEL_ID` 時維持原有行為（向下相容）
- 加入 `IntentsMessageContent` privileged intent，確保能讀到 Guild 訊息內容

**Non-Goals:**
- per-server 的允許頻道白名單（管理員 `/setchannel` 指令）
- 斜線指令（Slash commands）支援
- 多 Bot token / 多 guild 的複雜路由

## Decisions

### 決策 1：初始化條件放寬

**現況：**
```go
if discordToken != "" && discordChannel != "" {
    // 建立 discordSess
}
```

**改後：**
```go
if discordToken != "" {
    discordSess = newDiscordSessionManager(discordToken, discordChannel, workdir, logger.Logger)
    // discordChannel 可為空字串
    if im == nil {
        im = newIMManager(logger.Logger)
    }
    im.addAdapter(discordSess)
}
```

理由：channel ID 只影響 `onMessage` 的過濾邏輯，`DiscordSessionManager` 本身不需要它來啟動。

### 決策 2：DM vs Private Guild vs Public Guild 三分流

在 `onMessage` 加入頻道類型判斷：

```go
isDM := m.GuildID == ""
isPrivate := !isDM && d.isPrivateChannel(s, m.ChannelID)

if !isDM && !isPrivate {
    // Public Guild 頻道：需要 @mention
    mentioned := false
    for _, user := range m.Mentions {
        if user.ID == s.State.User.ID {
            mentioned = true
            break
        }
    }
    if !mentioned {
        return
    }
}
```

`GuildID == ""` 是 discordgo 中 DM 的標準判斷方式。Private channel 透過 channel permission overwrites 判斷（`@everyone` 的 `read_messages` 為 `false`）。

### 決策 2a：Private Channel Cache

查詢頻道是否為 private 需要 API call（`s.Channel(channelID)`），為避免每條訊息都打 API，在 `DiscordSessionManager` 加入 cache：

```go
type DiscordSessionManager struct {
    // ...existing fields...
    channelPrivate map[string]bool // channelID → isPrivate, cached
}

func (d *DiscordSessionManager) isPrivateChannel(s *discordgo.Session, channelID string) bool {
    d.mu.Lock()
    if v, ok := d.channelPrivate[channelID]; ok {
        d.mu.Unlock()
        return v
    }
    d.mu.Unlock()

    ch, err := s.Channel(channelID)
    if err != nil {
        return false // 查不到就當 public，保守處理
    }
    // 若 @everyone 的 ViewChannel 被明確拒絕，視為 private
    isPrivate := false
    for _, ow := range ch.PermissionOverwrites {
        if ow.ID == ch.GuildID && ow.Type == discordgo.PermissionOverwriteTypeRole {
            if ow.Deny&discordgo.PermissionViewChannel != 0 {
                isPrivate = true
            }
            break
        }
    }

    d.mu.Lock()
    d.channelPrivate[channelID] = isPrivate
    d.mu.Unlock()
    return isPrivate
}
```

Cache 永不過期（頻道類型很少變動）。若未來需要支援頻道類型變更，可加 TTL 或 channel update event 清除 cache。

理由：每個 channel 只在第一條訊息時查一次 API，之後走 cache，幾乎沒有額外開銷。

### 決策 3：IntentsMessageContent

現有 intents：
```go
session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
```

改後：
```go
session.Identify.Intents = discordgo.IntentsGuildMessages |
    discordgo.IntentsDirectMessages |
    discordgo.IntentsMessageContent
```

理由：Discord 2022 年起限制 Message Content，沒有此 privileged intent 時 `m.Content` 在非 mention 訊息中為空字串。雖然 Guild 頻道需要 @mention（此情況 content 會有值），但 DM 的 content 在沒有此 intent 時亦可能為空。統一加上此 intent 最安全。

**注意**：`IntentsMessageContent` 是 privileged intent，使用者需在 Discord Developer Portal → Bot → Privileged Gateway Intents 手動開啟。

### 決策 4：mention 後去除 mention prefix

當 Guild 頻道 @mention Bot 時，`m.Content` 會包含 `<@BOT_ID>` 前綴（例如 `<@1234567890> 你好`）。寫入 PTY 前應去除此前綴，避免 Claude 看到奇怪的輸入。

```go
content := m.Content
if !isDM {
    content = strings.TrimSpace(mentionRegexp.ReplaceAllString(content, ""))
}
```

## Risks / Trade-offs

- **Privileged Intent 需手動開啟** → 文件清楚說明；若未開啟，Guild DM 的 `m.Content` 為空，Bot 收到空訊息。可在 `onMessage` 加防禦：若 `m.Content` 為空則 skip。
- **無限頻道建立 session** → 每個頻道建一個 PTY，Server 很多時資源消耗增加。目前 scope 不解決此問題，接受此 trade-off。
- **向下相容性** → 設定 `DISCORD_CHANNEL_ID` 時舊邏輯不變，不影響現有使用者。

## Migration Plan

1. 使用者更新 Perch 後，只需將 `DISCORD_CHANNEL_ID` 從環境變數移除（或保留，繼續原有行為）
2. 若要開放所有頻道：前往 Discord Developer Portal 開啟 Message Content Intent
3. 重新邀請 Bot（若要用 @mention 模式，邀請時確保有 `Read Messages`、`Send Messages`、`Add Reactions`、`Read Message History` 權限）

## Open Questions

- 是否需要限制 Guild 訊息只能來自 @mention 還是也支援 prefix command（`!ask`）？目前決定只支援 @mention，更乾淨。
