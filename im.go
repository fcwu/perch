package main

import (
	"log/slog"
	"sync"
)

// IMConfig carries optional startup configuration for IM adapters.
type IMConfig struct{}

// IMAdapter is implemented by each IM platform (Discord, Telegram, …).
type IMAdapter interface {
	// Start begins listening for messages using the provided config.
	Start(cfg IMConfig) error
	// Stop shuts down the adapter gracefully.
	Stop()
}

// TextSender is implemented by IM adapters that can send an arbitrary text message
// to a specific channel by ID (used for out-of-band notifications like git sync errors).
type TextSender interface {
	SendToChannel(channelID string, msg string) error
}

// SessionProvider is implemented by IM adapters that expose viewable PTY sessions.
type SessionProvider interface {
	ListSessions() []SessionView
	SubscribeSession(channelID string) (<-chan []byte, func(), bool)
	ResizeSession(channelID string, cols, rows uint16)
	WriteSession(channelID string, data []byte) error
}

// IMManager owns all adapters and dispatches hook events.
type IMManager struct {
	mu       sync.Mutex
	adapters []IMAdapter
	logger   *slog.Logger
}

func newIMManager(logger *slog.Logger) *IMManager {
	return &IMManager{logger: logger}
}

func (m *IMManager) addAdapter(a IMAdapter) {
	m.mu.Lock()
	m.adapters = append(m.adapters, a)
	m.mu.Unlock()
}

func (m *IMManager) start(cfg IMConfig) {
	m.mu.Lock()
	adapters := make([]IMAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.Unlock()
	for _, a := range adapters {
		if err := a.Start(cfg); err != nil {
			m.logger.Error("IM adapter start failed", "err", err)
		}
	}
}

func (m *IMManager) stop() {
	m.mu.Lock()
	adapters := make([]IMAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.Unlock()
	for _, a := range adapters {
		a.Stop()
	}
}

// SendText sends msg to channelID via the first adapter that implements TextSender.
func (m *IMManager) SendText(channelID string, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.adapters {
		if ts, ok := a.(TextSender); ok {
			return ts.SendToChannel(channelID, msg)
		}
	}
	return nil
}

// Sessions returns the first SessionProvider found among adapters, or nil.
func (m *IMManager) Sessions() SessionProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.adapters {
		if sp, ok := a.(SessionProvider); ok {
			return sp
		}
	}
	return nil
}

