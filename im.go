package main

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
)

// HookEvent is the JSON payload Claude Code sends to hooks.
type HookEvent struct {
	EventName    string           `json:"hook_event_name"`
	SessionID    string           `json:"session_id"`
	ToolName     string           `json:"tool_name,omitempty"`
	ToolInput    json.RawMessage  `json:"tool_input,omitempty"`
	ToolResponse json.RawMessage  `json:"tool_response,omitempty"`
	IsError      bool             `json:"is_error,omitempty"`
	Transcript   []TranscriptMsg  `json:"transcript,omitempty"`
}

// TranscriptMsg is one entry in a Stop hook transcript.
type TranscriptMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// IMAdapter is implemented by each IM platform (Discord, Telegram, …).
type IMAdapter interface {
	// Start begins listening for messages and stores a ref to pty for write.
	Start(pty *PTYManager) error
	// Stop shuts down the adapter gracefully.
	Stop()
	// Notify dispatches a Claude hook event to the platform.
	// lastText is the extracted assistant response (non-empty only on Stop).
	Notify(event HookEvent, lastText string) error
}

// SessionProvider is implemented by IM adapters that expose viewable PTY sessions.
type SessionProvider interface {
	ListSessions() []SessionView
	SubscribeSession(channelID string) (<-chan []byte, func(), bool)
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

func (m *IMManager) start(pty *PTYManager) {
	m.mu.Lock()
	adapters := make([]IMAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.Unlock()
	for _, a := range adapters {
		if err := a.Start(pty); err != nil {
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

func (m *IMManager) notify(event HookEvent) {
	lastText := ""
	if event.EventName == "Stop" {
		lastText = extractLastAssistantText(event.Transcript)
	}
	m.mu.Lock()
	adapters := make([]IMAdapter, len(m.adapters))
	copy(adapters, m.adapters)
	m.mu.Unlock()
	for _, a := range adapters {
		if err := a.Notify(event, lastText); err != nil {
			m.logger.Warn("IM notify error", "err", err)
		}
	}
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

// extractLastAssistantText pulls the last assistant message text from a transcript.
func extractLastAssistantText(msgs []TranscriptMsg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return contentToString(msgs[i].Content)
		}
	}
	return ""
}

func contentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}
