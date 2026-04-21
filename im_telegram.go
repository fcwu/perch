package main

import (
	"log/slog"
	"strconv"
	"sync"
	"time"

	telebot "gopkg.in/telebot.v3"
)

type telegramPending struct {
	msg *telebot.Message
}

// TelegramAdapter listens on a Telegram chat and proxies messages to the PTY.
type TelegramAdapter struct {
	token  string
	chatID int64
	logger *slog.Logger

	mu   sync.Mutex
	bot  *telebot.Bot
	pty  *PTYManager
	last *telegramPending
}

func newTelegramAdapter(token string, chatID int64, logger *slog.Logger) *TelegramAdapter {
	return &TelegramAdapter{token: token, chatID: chatID, logger: logger}
}

func (t *TelegramAdapter) Start(cfg IMConfig) error {
	t.mu.Lock()
	t.pty = cfg.PTY
	t.mu.Unlock()

	bot, err := telebot.NewBot(telebot.Settings{
		Token:  t.token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return err
	}
	bot.Handle(telebot.OnText, t.onText)

	t.mu.Lock()
	t.bot = bot
	t.mu.Unlock()

	go bot.Start()
	t.logger.Info("Telegram bot connected")
	return nil
}

func (t *TelegramAdapter) Stop() {
	t.mu.Lock()
	b := t.bot
	t.mu.Unlock()
	if b != nil {
		b.Stop()
	}
}

func (t *TelegramAdapter) onText(c telebot.Context) error {
	if c.Chat().ID != t.chatID {
		return nil
	}

	t.mu.Lock()
	t.last = &telegramPending{msg: c.Message()}
	pty := t.pty
	t.mu.Unlock()

	if pty != nil {
		if err := pty.write([]byte(c.Text() + "\r")); err != nil {
			t.logger.Warn("Telegram PTY write failed", "chatID", c.Chat().ID, "err", err)
		}
	}
	return nil
}

func (t *TelegramAdapter) Notify(event HookEvent, lastText string) error {
	if event.EventName != "Stop" {
		return nil
	}

	t.mu.Lock()
	b := t.bot
	pending := t.last
	t.mu.Unlock()
	if b == nil || pending == nil {
		return nil
	}

	text := lastText
	if text == "" {
		text = "✓ Claude finished."
	}
	if _, err := b.Reply(pending.msg, text); err != nil {
		t.logger.Warn("Telegram reply failed", "err", err)
	}

	t.mu.Lock()
	t.last = nil
	t.mu.Unlock()
	return nil
}

// parseTelegramChatID parses TELEGRAM_CHAT_ID env string to int64.
func parseTelegramChatID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
