package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log/slog"
	"regexp"
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

// startPTYWatcher subscribes to PTY output and cycles "still working" emoji reactions
// on the pending message every 10 s of activity. Call stopPTYWatcher (or cancel the
// session) to tear it down.
func (sess *discordSession) startPTYWatcher(s *discordgo.Session, pending *discordPending, logger *slog.Logger) {
	sess.watcherMu.Lock()
	if sess.watcherCancel != nil {
		sess.watcherCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	sess.watcherCancel = cancel
	sess.watcherMu.Unlock()

	ptyCh, unsub := sess.pty.subscribe()
	go func() {
		defer unsub()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		var lastEmoji string
		emojiIdx := 0
		hadActivity := false
		for {
			select {
			case <-ctx.Done():
				if lastEmoji != "" {
					_ = s.MessageReactionRemove(pending.ChannelID, pending.MessageID, lastEmoji, "@me")
				}
				return
			case _, ok := <-ptyCh:
				if !ok {
					return
				}
				hadActivity = true
			case <-ticker.C:
				if !hadActivity {
					continue
				}
				hadActivity = false
				next := workingEmojis[emojiIdx%len(workingEmojis)]
				emojiIdx++
				if lastEmoji != "" {
					_ = s.MessageReactionRemove(pending.ChannelID, pending.MessageID, lastEmoji, "@me")
				}
				if err := s.MessageReactionAdd(pending.ChannelID, pending.MessageID, next); err != nil {
					logger.Warn("Discord working reaction add failed", "emoji", next, "err", err)
				}
				lastEmoji = next
			}
		}
	}()
}

// stopPTYWatcher cancels the running watcher (if any).
func (sess *discordSession) stopPTYWatcher() {
	sess.watcherMu.Lock()
	if sess.watcherCancel != nil {
		sess.watcherCancel()
		sess.watcherCancel = nil
	}
	sess.watcherMu.Unlock()
}

type discordPending struct {
	MessageID string
	ChannelID string
	GuildID   string
}

// discordSession is one per Discord channel: its own PTY + pending state.
type discordSession struct {
	channelID   string
	sessionUUID string
	pty         *PTYManager

	mu   sync.Mutex
	last *discordPending
	warm bool // true after the first message has been successfully written to the PTY

	watcherMu     sync.Mutex
	watcherCancel func() // non-nil while a PTY-change watcher goroutine is running
}

func newDiscordSession(channelID string, logger *slog.Logger, workdir string) *discordSession {
	pty := newPTYManager()
	go pty.start("claude", []string{"--permission-mode", "bypassPermissions", "--name", "discord:" + channelID}, workdir, logger,
		"PERCH_SESSION_TARGET=discord:"+channelID)
	return &discordSession{
		channelID: channelID,
		// sessionUUID is empty until the first hook event claims it.
		pty: pty,
	}
}

// SessionView is the JSON representation of a live Discord session.
type SessionView struct {
	ChannelID   string `json:"channel_id"`
	SessionUUID string `json:"session_uuid"`
}

// DiscordSessionManager listens on Discord and routes each channel to its own PTY.
type DiscordSessionManager struct {
	token              string
	allowedChannelID   string
	allowedDMUserIDs   map[string]struct{} // nil/empty = DM disabled
	logger             *slog.Logger
	workdir            string

	mu             sync.Mutex
	dgo            *discordgo.Session
	sessions       map[string]*discordSession // channelID → session
	channelPrivate map[string]bool            // channelID → isPrivate, cached
}

func newDiscordSessionManager(token, channelID string, allowedDMUsers []string, workdir string, logger *slog.Logger) *DiscordSessionManager {
	dmIDs := make(map[string]struct{}, len(allowedDMUsers))
	for _, id := range allowedDMUsers {
		dmIDs[id] = struct{}{}
	}
	return &DiscordSessionManager{
		token:            token,
		allowedChannelID: channelID,
		allowedDMUserIDs: dmIDs,
		logger:           logger,
		workdir:          workdir,
		sessions:         make(map[string]*discordSession),
		channelPrivate:   make(map[string]bool),
	}
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

func (d *DiscordSessionManager) Start(_ *PTYManager) error {
	// Pre-start the allowed channel's session so PTY is ready before first message.
	if d.allowedChannelID != "" {
		d.getOrCreateSession(d.allowedChannelID)
	}

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
	d.logger.Info("Discord bot connected (per-channel PTY mode)")
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
		sess.pty.stop()
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

	sess.mu.Lock()
	idle := sess.last == nil
	sess.last = &discordPending{
		MessageID: m.ID,
		ChannelID: m.ChannelID,
		GuildID:   m.GuildID,
	}
	sess.mu.Unlock()

	// If the session was idle, the previous sessionUUID may be stale (e.g.
	// claude exited without firing a Stop hook).  Clear it so the new claude
	// invocation can claim the session when its first hook event arrives.
	if idle {
		d.mu.Lock()
		if sess.sessionUUID != "" {
			d.logger.Info("Discord clearing stale sessionUUID", "channel", m.ChannelID, "uuid", sess.sessionUUID)
			sess.sessionUUID = ""
		}
		d.mu.Unlock()
	}

	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emojiEyes); err != nil {
		d.logger.Warn("Discord add reaction failed", "emoji", emojiEyes, "err", err)
	}

	// If this is a brand-new PTY session (empty framebuffer), Claude Code has not
	// If Claude Code is not yet warm (first message on this session), we must
	// wait for it to finish initialising before writing.  Writing too early
	// causes the welcome-screen TUI to consume the '\r' terminator, discarding
	// the message.
	//
	// We use sess.warm (set after the first successful write) rather than
	// checking framebuf length to avoid a race: the 👀 reaction Discord API
	// call above takes ~100 ms, during which the PTY goroutine may already
	// have produced output, making framebuf non-empty before we check it.
	sess.mu.Lock()
	needsWarm := !sess.warm
	sess.mu.Unlock()

	writeToPTY := func() {
		if err := sess.pty.write([]byte(content + "\r")); err != nil {
			d.logger.Warn("Discord PTY write failed", "channel", m.ChannelID, "msgID", m.ID, "err", err)
			return
		}
		sess.mu.Lock()
		sess.warm = true
		pending := sess.last
		sess.mu.Unlock()
		if pending != nil {
			sess.startPTYWatcher(s, pending, d.logger)
		}
	}
	if needsWarm {
		go func() {
			ch, unsub := sess.pty.subscribe()
			defer unsub()
			deadline := time.After(120 * time.Second)
			chunks := 0

			// Phase 1: wait for the Claude Code interactive TUI to be fully
			// rendered.  "bypass permissions" appears in the status bar only
			// after MCPs have loaded and the readline is active, making it a
			// reliable marker that the PTY is ready for input.
			// We also accept the raw ❯ prompt as a fallback for non-bypass modes.
			foundPrompt := false
			for !foundPrompt {
				select {
				case data := <-ch:
					chunks++
					s := string(data)
					if strings.Contains(s, "bypass permissions") || strings.Contains(s, "❯") {
						foundPrompt = true
						d.logger.Debug("Discord needsWarm: prompt detected", "channel", m.ChannelID, "chunk", chunks, "bytes", len(data))
					}
				case <-deadline:
					d.logger.Debug("Discord needsWarm: deadline before prompt", "channel", m.ChannelID, "chunks", chunks)
					writeToPTY()
					return
				}
			}

			// Phase 2: wait for output to settle (no new PTY data for 2 s).
			// This ensures any final rendering has finished before we inject
			// the user's message.
			stable := time.NewTimer(2 * time.Second)
			defer stable.Stop()
			for {
				select {
				case <-ch:
					// More output arrived — reset the stability window.
					if !stable.Stop() {
						select {
						case <-stable.C:
						default:
						}
					}
					stable.Reset(2 * time.Second)
				case <-stable.C:
					d.logger.Debug("Discord needsWarm: stable, writing", "channel", m.ChannelID)
					writeToPTY()
					return
				case <-deadline:
					d.logger.Debug("Discord needsWarm: deadline in phase2", "channel", m.ChannelID)
					writeToPTY()
					return
				}
			}
		}()
	} else {
		writeToPTY()
	}
}

func (d *DiscordSessionManager) getOrCreateSession(channelID string) *discordSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sess, ok := d.sessions[channelID]; ok {
		return sess
	}
	sess := newDiscordSession(channelID, d.logger, d.workdir)
	d.sessions[channelID] = sess
	return sess
}

func (d *DiscordSessionManager) Notify(event HookEvent, lastText string) error {
	d.mu.Lock()
	var target *discordSession
	for _, sess := range d.sessions {
		if sess.sessionUUID == event.SessionID {
			target = sess
			break
		}
		// Claim an unassigned session only when there is a pending message
		// (user-triggered via Discord or scheduler-triggered via OnScheduledFire).
		// This prevents main-PTY hook events from being routed to Discord.
		if sess.sessionUUID == "" {
			sess.mu.Lock()
			hasPending := sess.last != nil
			sess.mu.Unlock()
			if hasPending {
				sess.sessionUUID = event.SessionID
				target = sess
				break
			}
		}
	}
	dgo := d.dgo
	d.mu.Unlock()
	if target == nil {
		d.logger.Debug("Discord Notify: no session matched, dropping event",
			"event", event.EventName, "sessionID", event.SessionID)
		return nil
	}
	err := target.notify(dgo, event, lastText, d.logger)
	// After Stop, clear sessionUUID so the next conversation can be tracked.
	if event.EventName == "Stop" {
		d.mu.Lock()
		target.sessionUUID = ""
		d.mu.Unlock()
	}
	return err
}

func (sess *discordSession) notify(s *discordgo.Session, event HookEvent, lastText string, logger *slog.Logger) error {
	sess.mu.Lock()
	pending := sess.last
	sess.mu.Unlock()
	if s == nil {
		return nil
	}

	// Scheduler-triggered (no pending user message): only handle Stop,
	// send response directly to the channel without a reply reference.
	if pending == nil {
		if event.EventName != "Stop" {
			return nil
		}
		text := lastText
		if text == "" {
			text = "✓ Claude finished."
		}
		var lastErr error
		for _, chunk := range splitForDiscord(text) {
			if _, err := s.ChannelMessageSend(sess.channelID, chunk); err != nil {
				logger.Warn("Discord send autonomous reply failed", "err", err)
				lastErr = err
			}
		}
		return lastErr
	}

	switch event.EventName {
	case "PreToolUse":
		if err := s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiGear); err != nil {
			logger.Warn("Discord reaction add failed", "emoji", emojiGear, "err", err)
		}

	case "PostToolUse":
		if err := s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiGear, "@me"); err != nil {
			logger.Warn("Discord reaction remove failed", "emoji", emojiGear, "err", err)
		}
		if event.IsError {
			if err := s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiCross); err != nil {
				logger.Warn("Discord reaction add failed", "emoji", emojiCross, "err", err)
			}
		} else {
			if err := s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiCheck); err != nil {
				logger.Warn("Discord reaction add failed", "emoji", emojiCheck, "err", err)
			}
		}

	case "Stop":
		sess.stopPTYWatcher()

		if err := s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiEyes, "@me"); err != nil {
			logger.Warn("Discord reaction remove failed", "emoji", emojiEyes, "err", err)
		}
		if err := s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiGear, "@me"); err != nil {
			logger.Warn("Discord reaction remove failed", "emoji", emojiGear, "err", err)
		}
		if err := s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiSpeech); err != nil {
			logger.Warn("Discord reaction add failed", "emoji", emojiSpeech, "err", err)
		}

		text := lastText
		if text == "" {
			text = "✓ Claude finished."
		}
		chunks := splitForDiscord(text)
		logger.Info("Discord sending Stop reply", "channel", sess.channelID, "replyTo", pending.MessageID, "chunks", len(chunks))
		for i, chunk := range chunks {
			if i == 0 {
				_, err := s.ChannelMessageSendComplex(pending.ChannelID, &discordgo.MessageSend{
					Content:   chunk,
					Reference: &discordgo.MessageReference{MessageID: pending.MessageID},
				})
				if err != nil {
					logger.Warn("Discord send reply failed", "channel", sess.channelID, "err", err)
				}
			} else {
				if _, err := s.ChannelMessageSend(pending.ChannelID, chunk); err != nil {
					logger.Warn("Discord send continuation failed", "channel", sess.channelID, "part", i, "err", err)
				}
			}
		}

		sess.mu.Lock()
		sess.last = nil
		sess.mu.Unlock()
	}
	return nil
}

// ListSessions returns all active Discord sessions (implements SessionProvider).
func (d *DiscordSessionManager) ListSessions() []SessionView {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]SessionView, 0, len(d.sessions))
	for _, sess := range d.sessions {
		out = append(out, SessionView{
			ChannelID:   sess.channelID,
			SessionUUID: sess.sessionUUID,
		})
	}
	return out
}

// SubscribeSession returns a read channel for the PTY output of channelID.
func (d *DiscordSessionManager) SubscribeSession(channelID string) (<-chan []byte, func(), bool) {
	d.mu.Lock()
	sess, ok := d.sessions[channelID]
	d.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	ch, unsub := sess.pty.subscribe()
	return ch, unsub, true
}

// OnScheduledFire is called by the scheduler before writing a job message to a Discord PTY.
// It sends a header message to Discord so the scheduled run is visible, and stores the
// message ID as sess.last so Claude's reply threads back to it.
func (d *DiscordSessionManager) OnScheduledFire(target, message string) {
	const prefix = "discord:"
	if !strings.HasPrefix(target, prefix) {
		return
	}
	channelID := target[len(prefix):]

	d.mu.Lock()
	dgo := d.dgo
	d.mu.Unlock()
	if dgo == nil {
		return
	}

	sent, err := dgo.ChannelMessageSend(channelID, "📅 local schedule > "+message)
	if err != nil {
		d.logger.Warn("Discord schedule header send failed", "err", err)
		return
	}

	sess := d.getOrCreateSession(channelID)
	sess.mu.Lock()
	sess.last = &discordPending{
		MessageID: sent.ID,
		ChannelID: channelID,
	}
	sess.mu.Unlock()
}

// PTYForTarget returns the PTYManager for a session target string (e.g. "discord:<channelID>").
// Returns nil if the target is not a known Discord session.
func (d *DiscordSessionManager) PTYForTarget(target string) *PTYManager {
	const prefix = "discord:"
	if !strings.HasPrefix(target, prefix) {
		return nil
	}
	channelID := target[len(prefix):]
	d.mu.Lock()
	sess, ok := d.sessions[channelID]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	return sess.pty
}

// ResizeSession resizes the PTY for the given Discord channel.
func (d *DiscordSessionManager) ResizeSession(channelID string, cols, rows uint16) {
	d.mu.Lock()
	sess, ok := d.sessions[channelID]
	d.mu.Unlock()
	if ok {
		sess.pty.resize(cols, rows)
	}
}
