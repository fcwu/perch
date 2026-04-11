package main

import (
	"log/slog"
	"sync"

	"github.com/bwmarrin/discordgo"
)

const (
	emojiEyes     = "👀"
	emojiGear     = "⚙️"
	emojiCheck    = "✅"
	emojiCross    = "❌"
	emojiSpeech   = "💬"
)

type discordPending struct {
	MessageID string
	ChannelID string
	GuildID   string
}

// DiscordAdapter listens on a Discord channel and proxies messages to the PTY.
type DiscordAdapter struct {
	token     string
	channelID string
	logger    *slog.Logger

	mu      sync.Mutex
	session *discordgo.Session
	pty     *PTYManager
	last    *discordPending
}

func newDiscordAdapter(token, channelID string, logger *slog.Logger) *DiscordAdapter {
	return &DiscordAdapter{token: token, channelID: channelID, logger: logger}
}

func (d *DiscordAdapter) Start(pty *PTYManager) error {
	d.mu.Lock()
	d.pty = pty
	d.mu.Unlock()

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
	d.session = session
	d.mu.Unlock()
	d.logger.Info("Discord bot connected")
	return nil
}

func (d *DiscordAdapter) Stop() {
	d.mu.Lock()
	s := d.session
	d.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

func (d *DiscordAdapter) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	if m.ChannelID != d.channelID {
		return
	}

	d.mu.Lock()
	d.last = &discordPending{
		MessageID: m.ID,
		ChannelID: m.ChannelID,
		GuildID:   m.GuildID,
	}
	pty := d.pty
	d.mu.Unlock()

	// Add 👀 reaction to signal message received.
	d.react(s, m.ChannelID, m.ID, emojiEyes)

	if pty != nil {
		pty.write([]byte(m.Content + "\r"))
	}
}

func (d *DiscordAdapter) Notify(event HookEvent, lastText string) error {
	d.mu.Lock()
	s := d.session
	pending := d.last
	d.mu.Unlock()
	if s == nil || pending == nil {
		return nil
	}

	switch event.EventName {
	case "PreToolUse":
		d.react(s, pending.ChannelID, pending.MessageID, emojiGear)

	case "PostToolUse":
		d.unreact(s, pending.ChannelID, pending.MessageID, emojiGear)
		if event.IsError {
			d.react(s, pending.ChannelID, pending.MessageID, emojiCross)
		} else {
			d.react(s, pending.ChannelID, pending.MessageID, emojiCheck)
		}

	case "Stop":
		d.unreact(s, pending.ChannelID, pending.MessageID, emojiEyes)
		d.unreact(s, pending.ChannelID, pending.MessageID, emojiGear)
		d.react(s, pending.ChannelID, pending.MessageID, emojiSpeech)

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
			d.logger.Warn("Discord send reply failed", "err", err)
		}

		d.mu.Lock()
		d.last = nil
		d.mu.Unlock()
	}
	return nil
}

func (d *DiscordAdapter) react(s *discordgo.Session, channelID, messageID, emoji string) {
	if err := s.MessageReactionAdd(channelID, messageID, emoji); err != nil {
		d.logger.Warn("Discord add reaction failed", "emoji", emoji, "err", err)
	}
}

func (d *DiscordAdapter) unreact(s *discordgo.Session, channelID, messageID, emoji string) {
	if err := s.MessageReactionRemove(channelID, messageID, emoji, "@me"); err != nil {
		d.logger.Warn("Discord remove reaction failed", "emoji", emoji, "err", err)
	}
}
