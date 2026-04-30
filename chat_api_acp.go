package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// acpChatSession represents one active ACP-based chat-API session for a user.
type acpChatSession struct {
	userID         string
	username       string
	query          string
	conversationID string
	sessionID      string // store session UUID (set after InsertSession)
	poolKey        string

	mu     sync.Mutex
	jsonCh chan string
	done   bool
}

func newACPChatSession(userID, username, query, conversationID string) *acpChatSession {
	convID := conversationID
	if convID == "" {
		convID = "default"
	}
	return &acpChatSession{
		userID:         userID,
		username:       username,
		query:          query,
		conversationID: conversationID,
		sessionID:      newUUID(),
		poolKey:        "chat-api:" + userID + ":" + convID,
		jsonCh:         make(chan string, 256),
	}
}

func (s *acpChatSession) broadcastJSON(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	select {
	case s.jsonCh <- msg:
	default:
	}
}

func (s *acpChatSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.done = true
		close(s.jsonCh)
	}
}

// ChatSessionManager is the interface shared by PTY-based and ACP-based session managers.
// The server's chat handlers depend only on this interface.
type ChatSessionManager interface {
	StartSession(userID, username, query string, newConversation bool, conversationID string) error
	SubscribeSession(userID string) (<-chan []byte, func(), bool)
	SubscribeJSON(userID string) (<-chan string, func(), bool)
}

// ACPUserSessionManager is the ACP-based implementation of ChatSessionManager.
// Each user gets at most one active session at a time (same semantics as PTY mode).
// The ACP subprocess for a (user, conversation) pair is reused across turns.
type ACPUserSessionManager struct {
	pool     *ACPSessionPool
	store    *Store
	adminHub *ManagementHub
	logger   *slog.Logger
	timeout  time.Duration

	mu       sync.Mutex
	sessions map[string]*acpChatSession // userID → active session
}

func newACPUserSessionManager(workdir string, store *Store, adminHub *ManagementHub, logger *slog.Logger) *ACPUserSessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ACPUserSessionManager{
		pool:     newACPSessionPool("", workdir, logger),
		store:    store,
		adminHub: adminHub,
		logger:   logger,
		timeout:  userSessionTimeout,
		sessions: make(map[string]*acpChatSession),
	}
}

// StartSession starts an ACP prompt for userID. Returns 409-alike error if busy.
func (m *ACPUserSessionManager) StartSession(userID, username, query string, newConversation bool, conversationID string) error {
	m.mu.Lock()
	existing, ok := m.sessions[userID]
	if ok {
		existing.mu.Lock()
		running := !existing.done
		existing.mu.Unlock()
		if running {
			m.mu.Unlock()
			return &sessionConflictError{userID: userID}
		}
		delete(m.sessions, userID)
	}

	sess := newACPChatSession(userID, username, query, conversationID)
	m.sessions[userID] = sess
	m.mu.Unlock()

	// Build prompt with history if continuing a conversation.
	prompt := query
	if !newConversation && m.store != nil {
		since := time.Now().UnixMilli() - conversationWindow.Milliseconds()
		if history, err := m.store.GetRecentHistory(userID, since, conversationMaxTurns); err != nil {
			m.logger.Warn("ACP chat: GetRecentHistory failed", "err", err)
		} else {
			prompt = buildPrompt(history, query)
		}
	}

	go m.runPrompt(sess, prompt)
	return nil
}

func (m *ACPUserSessionManager) runPrompt(sess *acpChatSession, prompt string) {
	defer func() {
		// Clean up session entry after a retain window.
		time.AfterFunc(userSessionRetain, func() {
			m.mu.Lock()
			cur, ok := m.sessions[sess.userID]
			if ok && cur == sess {
				delete(m.sessions, sess.userID)
			}
			m.mu.Unlock()
		})
	}()

	// Record session start in store and notify admin hub.
	if m.store != nil {
		if sess.conversationID != "" {
			_ = m.store.InsertSessionWithConversation(sess.sessionID, sess.userID, sess.username, sess.query, sess.conversationID)
		} else {
			_ = m.store.InsertSession(sess.sessionID, sess.userID, sess.username, sess.query)
		}
	}
	if m.adminHub != nil {
		m.adminHub.SessionAdded(ManagementSessionView{
			ID:        sess.sessionID,
			Username:  sess.username,
			Query:     sess.query,
			Status:    "running",
			StartedAt: nowMs(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	proc, err := m.pool.Acquire(ctx, sess.poolKey)
	if err != nil {
		m.logger.Warn("ACP chat: acquire session", "userID", sess.userID, "err", err)
		errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		sess.broadcastJSON(string(errMsg))
		sess.close()
		if m.adminHub != nil {
			m.adminHub.SessionRemoved(sess.sessionID, "error")
		}
		return
	}
	defer m.pool.Release(sess.poolKey)

	onChunk := func(chunk string) {
		msg, _ := json.Marshal(map[string]string{"type": "chunk", "text": chunk})
		sess.broadcastJSON(string(msg))
	}
	onToolStart := func(toolName string) {
		if m.adminHub != nil {
			m.adminHub.SessionUpdated(sess.sessionID, toolName)
		}
		toolMsg, _ := json.Marshal(map[string]string{"type": "tool_start", "tool": toolName})
		sess.broadcastJSON(string(toolMsg))
	}
	onToolEnd := func() {
		if m.adminHub != nil {
			m.adminHub.SessionUpdated(sess.sessionID, "")
		}
	}

	response, err := proc.PromptWithChunks(ctx, prompt, onChunk, onToolStart, onToolEnd)
	if err != nil {
		errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		sess.broadcastJSON(string(errMsg))
	}

	// Record session completion.
	if m.store != nil {
		if sess.conversationID != "" {
			_ = m.store.UpdateSessionDoneAndTouch(sess.sessionID, response, sess.conversationID)
		} else {
			_ = m.store.UpdateSessionDone(sess.sessionID, response)
		}
	}
	if m.adminHub != nil {
		m.adminHub.SessionRemoved(sess.sessionID, "done")
	}

	doneMsg, _ := json.Marshal(map[string]string{"type": "done"})
	sess.broadcastJSON(string(doneMsg))
	sess.close()

	m.logger.Info("ACP chat: prompt done", "userID", sess.userID, "conv", sess.conversationID)
}

// SubscribeSession returns a nil byte channel (no raw PTY output in ACP mode).
// Returns ok=true as long as a session for userID exists.
func (m *ACPUserSessionManager) SubscribeSession(userID string) (<-chan []byte, func(), bool) {
	m.mu.Lock()
	_, ok := m.sessions[userID]
	m.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	return nil, func() {}, true
}

// SubscribeJSON returns a channel that receives JSON event strings for the user's session.
func (m *ACPUserSessionManager) SubscribeJSON(userID string) (<-chan string, func(), bool) {
	m.mu.Lock()
	sess, ok := m.sessions[userID]
	m.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	return sess.jsonCh, func() {}, true
}
