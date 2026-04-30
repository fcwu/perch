package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// mentionRe matches Discord user mention tokens like <@123456789> or <@!123456789>.
var mentionRe = regexp.MustCompile(`<@!?\d+>`)

const (
	emojiEyes   = "👀"
	emojiGear   = "⚙️"
	emojiCheck  = "✅"
	emojiCross  = "❌"
	emojiSpeech = "💬"

	discordMaxLen = 1900 // leave room below Discord's 2000-char hard limit
)

// workingEmojis are cycled as "still working" reactions when PTY output is detected.
var workingEmojis = []string{"⌨️", "💭", "✍️"}

// tableBlockRe matches two or more consecutive Markdown table lines (starting with |).
var tableBlockRe = regexp.MustCompile(`(?m)(?:^\|[^\n]*\n){2,}`)

// channelSessionID derives a deterministic UUID v5-like string from a Discord channel ID.
func channelSessionID(channelID string) string {
	h := sha1.Sum([]byte("perch-discord-v1:" + channelID))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// displayWidth returns the terminal display width of s, counting CJK/wide runes as 2.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// isWideRune reports whether r occupies two terminal columns (CJK and fullwidth ranges).
func isWideRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals / Kangxi
		(r >= 0x3040 && r <= 0x33FF) || // Hiragana, Katakana, CJK Symbols
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE10 && r <= 0xFE4F) || // Vertical / CJK Compatibility Forms
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth ASCII
		(r >= 0xFFE0 && r <= 0xFFE6) // Fullwidth Signs
}

// padRight pads s on the right with spaces to reach the target display width.
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

// isSepCell reports whether a table cell is a Markdown separator (e.g. ---, :---:).
func isSepCell(s string) bool {
	return strings.ContainsRune(s, '-') && strings.Trim(s, "-: ") == ""
}

// alignTable re-aligns the column widths of a Markdown table block so that
// | characters line up. Wide (CJK) characters are counted as 2 columns.
func alignTable(table string) string {
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) == 0 {
		return table
	}

	// Parse each line into trimmed cells (drop the leading/trailing empty parts
	// that come from splitting "| a | b |" on "|").
	rows := make([][]string, len(lines))
	for i, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			parts = parts[1 : len(parts)-1]
		}
		cells := make([]string, len(parts))
		for j, p := range parts {
			cells[j] = strings.TrimSpace(p)
		}
		rows[i] = cells
	}

	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return table
	}

	// Determine column widths: separator rows contribute ≥ 3 dashes.
	widths := make([]int, maxCols)
	for _, row := range rows {
		sep := len(row) > 0
		for _, cell := range row {
			if !isSepCell(cell) {
				sep = false
				break
			}
		}
		for j, cell := range row {
			if j >= maxCols {
				break
			}
			w := displayWidth(cell)
			if sep && w < 3 {
				w = 3
			}
			if w > widths[j] {
				widths[j] = w
			}
		}
	}

	// Reconstruct with aligned columns.
	var sb strings.Builder
	for i, row := range rows {
		sep := len(row) > 0
		for _, cell := range row {
			if !isSepCell(cell) {
				sep = false
				break
			}
		}
		sb.WriteString("|")
		for j := 0; j < maxCols; j++ {
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			w := widths[j]
			if sep {
				sb.WriteString(" " + strings.Repeat("-", w) + " |")
			} else {
				sb.WriteString(" " + padRight(cell, w) + " |")
			}
		}
		if i < len(rows)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// convertTablesToCodeBlocks wraps Markdown table blocks in ``` for nicer Discord rendering,
// and re-aligns columns so | characters line up.
func convertTablesToCodeBlocks(text string) string {
	return tableBlockRe.ReplaceAllStringFunc(text, func(m string) string {
		return "```\n" + alignTable(m) + "\n```\n"
	})
}

// splitForDiscord converts tables and splits text into ≤ discordMaxLen chunks.
// It never splits inside a ``` code block.
func splitForDiscord(text string) []string {
	text = convertTablesToCodeBlocks(text)
	if len(text) <= discordMaxLen {
		return []string{text}
	}
	var chunks []string
	lines := strings.Split(text, "\n")
	var buf strings.Builder
	inCode := false
	flush := func() {
		s := strings.TrimRight(buf.String(), "\n")
		if s != "" {
			chunks = append(chunks, s)
		}
		buf.Reset()
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
		}
		add := line + "\n"
		if !inCode && buf.Len() > 0 && buf.Len()+len(add) > discordMaxLen {
			flush()
		}
		buf.WriteString(add)
	}
	flush()
	return chunks
}


type discordPending struct {
	MessageID string
	ChannelID string
	GuildID   string
	AutoReply bool
}

// discordSession is one per Discord channel; all sessions use ACP subprocess mode.
type discordSession struct {
	channelID   string
	runtime     AgentRuntime
	sessionUUID string
	acpProcess  *ACPProcess // lazy-started on first Prompt()

	mu   sync.Mutex
	last *discordPending
}

func newDiscordSession(runtime AgentRuntime, channelID string, executable, workdir string, logger *slog.Logger) *discordSession {
	return &discordSession{
		channelID:  channelID,
		runtime:    runtime,
		acpProcess: NewACPProcess(executable, workdir, logger),
	}
}

// SessionView is the JSON representation of a live Discord session.
type SessionView struct {
	ChannelID   string `json:"channel_id"`
	SessionUUID string `json:"session_uuid"`
}

// DiscordSessionManager listens on Discord and routes each channel to its own ACP session.
type DiscordSessionManager struct {
	runtime          AgentRuntime
	token            string
	allowedChannelID string
	allowedDMUserIDs map[string]struct{} // nil/empty = DM disabled
	logger           *slog.Logger
	workdir          string

	mu             sync.Mutex
	acpExecutable  string // path to claude-agent-acp binary (default from ACP_EXECUTABLE / "claude-agent-acp")
	dgo            *discordgo.Session
	sessions       map[string]*discordSession // channelID → session
	channelPrivate map[string]bool            // channelID → isPrivate, cached
}

func newDiscordSessionManager(runtime AgentRuntime, token, channelID string, allowedDMUsers []string, workdir string, logger *slog.Logger) *DiscordSessionManager {
	dmIDs := make(map[string]struct{}, len(allowedDMUsers))
	for _, id := range allowedDMUsers {
		dmIDs[id] = struct{}{}
	}
	return &DiscordSessionManager{
		runtime:          runtime,
		token:            token,
		allowedChannelID: channelID,
		allowedDMUserIDs: dmIDs,
		logger:           logger,
		workdir:          workdir,
		sessions:         make(map[string]*discordSession),
		channelPrivate:   make(map[string]bool),
	}
}

// acpRunTimeout reads ACP_RUN_TIMEOUT (seconds) or returns the default 5 minutes.
func acpRunTimeout() time.Duration {
	if v := os.Getenv("ACP_RUN_TIMEOUT"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 5 * time.Minute
}

// isPrivateChannel returns true if the channel is not visible to @everyone.
// The result is cached so the Discord API is called at most once per channel.
func (d *DiscordSessionManager) isPrivateChannel(s *discordgo.Session, channelID string) bool {
	d.mu.Lock()
	if v, ok := d.channelPrivate[channelID]; ok {
		d.mu.Unlock()
		return v
	}
	d.mu.Unlock()

	ch, err := s.Channel(channelID)
	if err != nil {
		d.logger.Warn("Discord isPrivateChannel: channel lookup failed", "channel", channelID, "err", err)
		return false // treat as public on error
	}
	isPrivate := false
	for _, ow := range ch.PermissionOverwrites {
		// @everyone role has the same ID as the guild
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

func (d *DiscordSessionManager) Start(_ IMConfig) error {
	session, err := discordgo.New("Bot " + d.token)
	if err != nil {
		return err
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	session.AddHandler(d.onMessage)
	if err := session.Open(); err != nil {
		return err
	}
	d.mu.Lock()
	d.dgo = session
	d.mu.Unlock()
	d.logger.Info("Discord bot connected (ACP mode)")
	return nil
}

func (d *DiscordSessionManager) Stop() {
	d.mu.Lock()
	s := d.dgo
	sessions := make([]*discordSession, 0, len(d.sessions))
	for _, sess := range d.sessions {
		sessions = append(sessions, sess)
	}
	d.mu.Unlock()

	if s != nil {
		s.Close()
	}
	for _, sess := range sessions {
		if sess.acpProcess != nil {
			sess.acpProcess.Stop()
		}
	}
}

func (d *DiscordSessionManager) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	d.logger.Debug("Discord onMessage received", "channel", m.ChannelID, "guild", m.GuildID, "author", m.Author.Username, "content", m.Content, "mentions", len(m.Mentions))
	if m.Author == nil || m.Author.Bot {
		return
	}
	if d.allowedChannelID != "" && m.ChannelID != d.allowedChannelID {
		return
	}

	isDM := m.GuildID == ""
	if isDM {
		// DM is deny-by-default: only respond if the sender is in the allowlist.
		if _, ok := d.allowedDMUserIDs[m.Author.ID]; !ok {
			return
		}
	}
	isPrivate := !isDM && d.isPrivateChannel(s, m.ChannelID)

	var botID string
	if s.State.User != nil {
		botID = s.State.User.ID
	}
	d.logger.Debug("Discord onMessage routing", "isDM", isDM, "isPrivate", isPrivate, "botID", botID, "mentions", len(m.Mentions))

	content := m.Content
	if !isDM && !isPrivate {
		// Public Guild channel: require @mention.
		// Discord may not populate m.Mentions for bots without Message Content
		// Intent fully active, so also check the raw content for <@BOTID> or
		// <@!BOTID> tokens as a reliable fallback.
		mentioned := false
		for _, user := range m.Mentions {
			if s.State.User != nil && user.ID == s.State.User.ID {
				mentioned = true
				break
			}
		}
		if !mentioned && botID != "" {
			mentioned = strings.Contains(content, "<@"+botID+">") ||
				strings.Contains(content, "<@!"+botID+">")
		}
		d.logger.Debug("Discord onMessage public channel check", "mentioned", mentioned, "stateUser_nil", s.State.User == nil, "botID", botID)
		if !mentioned {
			return
		}
		// Strip all mention prefixes (e.g. "<@1234567890> ")
		content = strings.TrimSpace(mentionRe.ReplaceAllString(content, ""))
		if content == "" {
			return
		}
	}

	sess := d.getOrCreateSession(m.ChannelID)
	go sess.handleWithACP(s, m.ChannelID, m.ID, content, d.logger)
}

// handleWithACP sends message to the claude-agent-acp subprocess and routes the reply to Discord.
// It runs in its own goroutine and never returns an error (logs instead).
func (sess *discordSession) handleWithACP(s *discordgo.Session, channelID, messageID, content string, logger *slog.Logger) {
	// One message at a time per session — drop concurrent duplicates.
	sess.mu.Lock()
	if sess.last != nil {
		sess.mu.Unlock()
		logger.Debug("Discord ACP: session busy, dropping duplicate", "channel", channelID, "msgID", messageID)
		return
	}
	sess.last = &discordPending{MessageID: messageID, ChannelID: channelID}
	sess.mu.Unlock()
	defer func() {
		sess.mu.Lock()
		if sess.last != nil && sess.last.MessageID == messageID {
			sess.last = nil
		}
		sess.mu.Unlock()
	}()

	if err := s.MessageReactionAdd(channelID, messageID, emojiEyes); err != nil {
		logger.Warn("Discord ACP: add 👀 reaction failed", "err", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), acpRunTimeout())
	defer cancel()

	// Ensure the subprocess is running (lazy start / auto-recovery after crash).
	if err := sess.acpProcess.EnsureRunning(ctx); err != nil {
		logger.Warn("Discord ACP: process start failed", "channel", channelID, "err", err)
		_ = s.MessageReactionRemove(channelID, messageID, emojiEyes, "@me")
		_ = s.MessageReactionAdd(channelID, messageID, emojiCross)
		_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:   "❌ Agent unavailable: " + err.Error(),
			Reference: &discordgo.MessageReference{MessageID: messageID},
		})
		return
	}

	text, err := sess.acpProcess.Prompt(ctx, content)
	_ = s.MessageReactionRemove(channelID, messageID, emojiEyes, "@me")

	if err != nil {
		logger.Warn("Discord ACP: prompt failed", "channel", channelID, "err", err)
		_ = s.MessageReactionAdd(channelID, messageID, emojiCross)
		errMsg := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = "⏱️ Agent timed out."
		}
		_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:   "❌ " + errMsg,
			Reference: &discordgo.MessageReference{MessageID: messageID},
		})
		return
	}

	_ = s.MessageReactionAdd(channelID, messageID, emojiSpeech)
	if text == "" {
		return // nothing to send
	}
	chunks := splitForDiscord(text)
	logger.Info("Discord ACP: sending reply", "channel", channelID, "replyTo", messageID, "chunks", len(chunks))
	for i, chunk := range chunks {
		if i == 0 {
			_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content:   chunk,
				Reference: &discordgo.MessageReference{MessageID: messageID},
			})
			if err != nil {
				logger.Warn("Discord ACP: send reply failed", "channel", channelID, "err", err)
			}
			continue
		}
		if _, err := s.ChannelMessageSend(channelID, chunk); err != nil {
			logger.Warn("Discord ACP: send continuation failed", "channel", channelID, "part", i, "err", err)
		}
	}
}

func (d *DiscordSessionManager) getOrCreateSession(channelID string) *discordSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sess, ok := d.sessions[channelID]; ok {
		return sess
	}
	sess := newDiscordSession(d.runtime, channelID, d.acpExecutable, d.workdir, d.logger)
	d.sessions[channelID] = sess
	return sess
}

// ListSessions is a no-op since all Discord sessions are ACP (no viewable PTY stream).
func (d *DiscordSessionManager) ListSessions() []SessionView {
	return nil
}

// SubscribeSession always returns false — Discord sessions use ACP, not PTY.
func (d *DiscordSessionManager) SubscribeSession(_ string) (<-chan []byte, func(), bool) {
	return nil, nil, false
}

// DispatchScheduled handles a scheduler-fired job for a Discord channel target.
// Posts a visible header then routes the prompt via the channel's ACP session.
// Returns true when handled, false if the target is not a Discord channel.
func (d *DiscordSessionManager) DispatchScheduled(target, message string) bool {
	channelID, ok := parseDiscordTarget(target)
	if !ok {
		return false
	}

	d.mu.Lock()
	dgo := d.dgo
	d.mu.Unlock()
	if dgo == nil {
		return false
	}

	sent, err := dgo.ChannelMessageSend(channelID, "📅 local schedule > "+message)
	if err != nil {
		d.logger.Warn("Discord schedule header send failed", "channel", channelID, "err", err)
		return false
	}

	sess := d.getOrCreateSession(channelID)
	go sess.handleWithACP(dgo, channelID, sent.ID, message, d.logger)
	return true
}

// parseDiscordTarget extracts the channel ID from a scheduler target string.
// Accepts both the canonical "discord:<channelID>" form and the looser
// "discord:channel:<channelID>" form that mirrors Discord's <#channelID>
// mention syntax (T31/T32: tolerant parsing prevents 400 "not snowflake").
func parseDiscordTarget(target string) (string, bool) {
	const prefix = "discord:"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(target[len(prefix):], "channel:")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// ResizeSession is a no-op (Discord sessions use ACP, not PTY).
func (d *DiscordSessionManager) ResizeSession(_ string, _, _ uint16) {}

// SendToChannel sends a plain-text message to a Discord channel by ID.
// It is independent of allowedChannelID and intended for out-of-band notifications.
func (d *DiscordSessionManager) SendToChannel(channelID string, msg string) error {
	d.mu.Lock()
	dgo := d.dgo
	d.mu.Unlock()
	if dgo == nil {
		return fmt.Errorf("discord session not ready")
	}
	_, err := dgo.ChannelMessageSend(channelID, msg)
	return err
}

// WriteSession always returns an error — Discord sessions use ACP, not PTY.
func (d *DiscordSessionManager) WriteSession(channelID string, _ []byte) error {
	d.mu.Lock()
	_, ok := d.sessions[channelID]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("session not found: %s", channelID)
	}
	return fmt.Errorf("session %s uses ACP (no writable PTY)", channelID)
}
