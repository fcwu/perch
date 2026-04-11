package main

import (
	"crypto/sha1"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bwmarrin/discordgo"
)

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
}

func newDiscordSession(channelID string, logger *slog.Logger, workdir string) *discordSession {
	uuid := channelSessionID(channelID)
	pty := newPTYManager()
	go pty.start("claude", []string{"--session-id", uuid, "--name", "discord:" + channelID}, workdir, logger)
	return &discordSession{
		channelID:   channelID,
		sessionUUID: uuid,
		pty:         pty,
	}
}

// SessionView is the JSON representation of a live Discord session.
type SessionView struct {
	ChannelID   string `json:"channel_id"`
	SessionUUID string `json:"session_uuid"`
}

// DiscordSessionManager listens on Discord and routes each channel to its own PTY.
type DiscordSessionManager struct {
	token            string
	allowedChannelID string
	logger           *slog.Logger
	workdir          string

	mu       sync.Mutex
	dgo      *discordgo.Session
	sessions map[string]*discordSession // channelID → session
}

func newDiscordSessionManager(token, channelID, workdir string, logger *slog.Logger) *DiscordSessionManager {
	return &DiscordSessionManager{
		token:            token,
		allowedChannelID: channelID,
		logger:           logger,
		workdir:          workdir,
		sessions:         make(map[string]*discordSession),
	}
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
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
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
	if m.Author == nil || m.Author.Bot {
		return
	}
	if d.allowedChannelID != "" && m.ChannelID != d.allowedChannelID {
		return
	}

	sess := d.getOrCreateSession(m.ChannelID)

	sess.mu.Lock()
	sess.last = &discordPending{
		MessageID: m.ID,
		ChannelID: m.ChannelID,
		GuildID:   m.GuildID,
	}
	sess.mu.Unlock()

	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emojiEyes); err != nil {
		d.logger.Warn("Discord add reaction failed", "emoji", emojiEyes, "err", err)
	}
	sess.pty.write([]byte(m.Content + "\r"))
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
	}
	dgo := d.dgo
	d.mu.Unlock()
	if target == nil {
		return nil
	}
	return target.notify(dgo, event, lastText, d.logger)
}

func (sess *discordSession) notify(s *discordgo.Session, event HookEvent, lastText string, logger *slog.Logger) error {
	sess.mu.Lock()
	pending := sess.last
	sess.mu.Unlock()
	if s == nil || pending == nil {
		return nil
	}

	switch event.EventName {
	case "PreToolUse":
		s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiGear)

	case "PostToolUse":
		s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiGear, "@me")
		if event.IsError {
			s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiCross)
		} else {
			s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiCheck)
		}

	case "Stop":
		s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiEyes, "@me")
		s.MessageReactionRemove(pending.ChannelID, pending.MessageID, emojiGear, "@me")
		s.MessageReactionAdd(pending.ChannelID, pending.MessageID, emojiSpeech)

		text := lastText
		if text == "" {
			text = "✓ Claude finished."
		}
		if len(text) > 1900 {
			text = text[:1900] + "\n…(truncated)"
		}
		_, err := s.ChannelMessageSendComplex(pending.ChannelID, &discordgo.MessageSend{
			Content:   text,
			Reference: &discordgo.MessageReference{MessageID: pending.MessageID},
		})
		if err != nil {
			logger.Warn("Discord send reply failed", "err", err)
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
