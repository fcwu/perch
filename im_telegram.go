package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	telebot "gopkg.in/telebot.v3"
)

const (
	telegramMaxLen   = 4000 // conservative limit below Telegram's 4096-char hard cap
	telegramPoolKey  = "telegram:chat:"
	telegramTypingHz = 4 * time.Second // how often to send "typing…" action
)

// TelegramAdapter listens on a Telegram chat and proxies messages via ACP.
type TelegramAdapter struct {
	token  string
	chatID int64
	logger *slog.Logger
	pool   *ACPSessionPool

	mu  sync.Mutex
	bot *telebot.Bot
}

func newTelegramAdapter(runtime AgentRuntime, token string, chatID int64, workdir string, logger *slog.Logger) *TelegramAdapter {
	pool := newACPSessionPool(runtime.ACPExecutable, runtime.ACPArgs, workdir, logger)
	return &TelegramAdapter{
		token:  token,
		chatID: chatID,
		logger: logger,
		pool:   pool,
	}
}

func (t *TelegramAdapter) Start(cfg IMConfig) error {
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
	t.logger.Info("Telegram bot connected (ACP mode)")
	return nil
}

func (t *TelegramAdapter) Stop() {
	t.mu.Lock()
	b := t.bot
	t.mu.Unlock()
	if b != nil {
		b.Stop()
	}
	t.pool.StopAll()
}

func (t *TelegramAdapter) onText(c telebot.Context) error {
	if c.Chat().ID != t.chatID {
		return nil
	}
	text := c.Text()
	chatID := c.Chat().ID
	msg := c.Message()

	go t.handleWithACP(chatID, msg, text)
	return nil
}

func (t *TelegramAdapter) handleWithACP(chatID int64, msg *telebot.Message, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), acpRunTimeout())
	defer cancel()

	key := telegramPoolKey + strconv.FormatInt(chatID, 10)
	proc, err := t.pool.Acquire(ctx, key)
	if err != nil {
		t.logger.Warn("Telegram ACP: acquire session failed", "chatID", chatID, "err", err)
		t.replyError(msg, "❌ Agent unavailable: "+err.Error())
		return
	}
	defer t.pool.Release(key)

	// Send typing action periodically while waiting.
	typingStop := make(chan struct{})
	go func() {
		t.mu.Lock()
		bot := t.bot
		t.mu.Unlock()
		if bot == nil {
			return
		}
		ticker := time.NewTicker(telegramTypingHz)
		defer ticker.Stop()
		for {
			select {
			case <-typingStop:
				return
			case <-ticker.C:
				_ = bot.Notify(msg.Chat, telebot.Typing)
			}
		}
	}()

	reply, err := proc.Prompt(ctx, text)
	close(typingStop)

	if err != nil {
		t.logger.Warn("Telegram ACP: prompt failed", "chatID", chatID, "err", err)
		errMsg := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = "⏱️ Agent timed out."
		}
		t.replyError(msg, "❌ "+errMsg)
		return
	}

	if reply == "" {
		return
	}

	t.mu.Lock()
	bot := t.bot
	t.mu.Unlock()
	if bot == nil {
		return
	}

	for i, chunk := range splitForTelegram(reply) {
		var err error
		if i == 0 {
			_, err = bot.Reply(msg, chunk, telebot.ModeMarkdown)
			if err != nil {
				// Retry without markdown parsing if it failed (e.g. bad markdown).
				_, err = bot.Reply(msg, chunk)
			}
		} else {
			_, err = bot.Send(msg.Chat, chunk)
		}
		if err != nil {
			t.logger.Warn("Telegram ACP: send reply failed", "chatID", chatID, "part", i, "err", err)
		}
	}
}

func (t *TelegramAdapter) replyError(msg *telebot.Message, text string) {
	t.mu.Lock()
	bot := t.bot
	t.mu.Unlock()
	if bot == nil {
		return
	}
	if _, err := bot.Reply(msg, text); err != nil {
		t.logger.Warn("Telegram ACP: send error reply failed", "err", err)
	}
}

// splitForTelegram splits text into chunks of at most telegramMaxLen bytes,
// preferring to split at newlines.
func splitForTelegram(text string) []string {
	if len(text) <= telegramMaxLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if len(text) <= telegramMaxLen {
			chunks = append(chunks, text)
			break
		}
		cut := telegramMaxLen
		// Try to split at a newline within the last 200 bytes of the window.
		if idx := strings.LastIndex(text[:cut], "\n"); idx > cut-200 {
			cut = idx + 1
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}

// parseTelegramChatID parses TELEGRAM_CHAT_ID env string to int64.
func parseTelegramChatID(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parseTelegramChatID: %w", err)
	}
	return v, nil
}
