package main

import (
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
)

// channelSessionID derives a deterministic UUID v5-like string from a Discord channel ID.
func channelSessionID(channelID string) string {
	h := sha1.Sum([]byte("perch-discord-v1:" + channelID))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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
		sess.mu.Unlock()
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
		if len(text) > 1900 {
			text = text[:1900] + "\n…(truncated)"
		}
		_, err := s.ChannelMessageSend(sess.channelID, text)
		if err != nil {
			logger.Warn("Discord send autonomous reply failed", "err", err)
		}
		return err
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
		if len(text) > 1900 {
			text = text[:1900] + "\n…(truncated)"
		}
		logger.Info("Discord sending Stop reply", "channel", sess.channelID, "replyTo", pending.MessageID, "textLen", len(text))
		_, err := s.ChannelMessageSendComplex(pending.ChannelID, &discordgo.MessageSend{
			Content:   text,
			Reference: &discordgo.MessageReference{MessageID: pending.MessageID},
		})
		if err != nil {
			logger.Warn("Discord send reply failed", "channel", sess.channelID, "err", err)
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
