package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type userSessionStatus int

const (
	userSessionRunning   userSessionStatus = iota
	userSessionCompleted                   // PTY exited, output retained briefly
)

const (
	userSessionTimeout  = 5 * time.Minute
	userSessionRetain   = 5 * time.Minute
	conversationWindow   = 24 * time.Hour
	conversationMaxTurns = 20
)

// buildPrompt prepends serialised conversation history to query.
// Returns the raw query when history is empty.
func buildPrompt(history []ConversationTurn, query string) string {
	if len(history) == 0 {
		return query
	}
	var sb strings.Builder
	sb.WriteString("<conversation_history>\n")
	for _, t := range history {
		sb.WriteString("User: ")
		sb.WriteString(t.Query)
		sb.WriteString("\nAssistant: ")
		sb.WriteString(t.Response)
		sb.WriteString("\n")
	}
	sb.WriteString("</conversation_history>\n\n")
	sb.WriteString(query)
	return sb.String()
}

// userSession is one per authenticated user: an independent OpenCode PTY session.
type userSession struct {
	userID         string
	username       string
	conversationID string // conversation this session belongs to (empty if none)
	query          string // original user query (stored in DB, shown in admin)
	prompt         string // final injected prompt sent to the agent (may include history)
	startedAt      int64

	pty    *PTYManager
	status userSessionStatus

	mu        sync.Mutex
	jsonSubs  map[int]chan string
	nextSubID int

	cancel      context.CancelFunc
	completedAt time.Time
}

func newUserSession(userID, username, query string) *userSession {
	return &userSession{
		userID:    userID,
		username:  username,
		query:     query,
		startedAt: time.Now().UnixMilli(),
		pty:       newPTYManager(),
		jsonSubs:  make(map[int]chan string),
	}
}

// subscribeJSON registers a new JSON event subscriber and returns the channel and unsubscribe func.
func (s *userSession) subscribeJSON() (<-chan string, func()) {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	ch := make(chan string, 64)
	s.jsonSubs[id] = ch
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.jsonSubs, id)
		s.mu.Unlock()
	}
}

// broadcastJSON sends a JSON event string to all subscribers (non-blocking).
func (s *userSession) broadcastJSON(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.jsonSubs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (s *userSession) closeAllJSON() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.jsonSubs {
		close(ch)
	}
	s.jsonSubs = make(map[int]chan string)
}

// UserSessionManager manages per-user OpenCode PTY sessions.
type UserSessionManager struct {
	runtime   AgentRuntime
	workdir   string
	logger    *slog.Logger
	store     *Store
	adminHub  *ManagementHub

	mu       sync.Mutex
	sessions map[string]*userSession // userID → session
}

func newUserSessionManager(runtime AgentRuntime, workdir string, logger *slog.Logger, store *Store, adminHub *ManagementHub) *UserSessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserSessionManager{
		runtime:  runtime,
		workdir:  workdir,
		logger:   logger,
		store:    store,
		adminHub: adminHub,
		sessions: make(map[string]*userSession),
	}
}

// StartSession creates a new OpenCode session for the user.
// When newConversation is false, recent history is prepended to the query.
// conversationID links the session to an existing conversation (may be empty).
// Returns error with HTTP 409 status hint if a session is already running.
func (m *UserSessionManager) StartSession(userID, username, query string, newConversation bool, conversationID string) error {
	m.mu.Lock()
	existing, ok := m.sessions[userID]
	if ok {
		existing.mu.Lock()
		st := existing.status
		existing.mu.Unlock()
		if st == userSessionRunning {
			m.mu.Unlock()
			return &sessionConflictError{userID: userID}
		}
		// Previous session completed — remove it.
		delete(m.sessions, userID)
	}

	prompt := query
	if !newConversation && m.store != nil {
		since := time.Now().UnixMilli() - conversationWindow.Milliseconds()
		if history, err := m.store.GetRecentHistory(userID, since, conversationMaxTurns); err != nil {
			m.logger.Warn("GetRecentHistory failed, proceeding without history", "err", err)
		} else {
			prompt = buildPrompt(history, query)
		}
	}

	sess := newUserSession(userID, username, query)
	sess.prompt = prompt
	sess.conversationID = conversationID
	m.sessions[userID] = sess
	m.mu.Unlock()

	cmd, args := m.runtime.RunAgent("as-query", prompt, m.workdir)

	ctx, cancel := context.WithTimeout(context.Background(), userSessionTimeout)
	sess.cancel = cancel

	go func() {
		defer cancel()
		// startOnce runs the PTY once and closes p.done on exit.
		sess.pty.startOnce(cmd, args, m.workdir, m.logger)

		m.logger.Info("UserSession PTY exited", "userID", userID)

		// Broadcast `done` only now that the PTY has fully drained, so any
		// trailing bytes claude wrote between the Stop hook firing and the
		// process exiting reach SSE/WS clients before the stream closes.
		// The Stop hook handler intentionally does not emit `done`; it just
		// records completion state. This also covers the no-hooks case
		// (MT07/T52) where Stop never arrives — the frontend still unblocks.
		sess.mu.Lock()
		alreadyCompleted := sess.status == userSessionCompleted
		sess.status = userSessionCompleted
		sess.completedAt = time.Now()
		sess.mu.Unlock()

		doneMsg, _ := json.Marshal(map[string]string{"type": "done"})
		sess.broadcastJSON(string(doneMsg))

		if !alreadyCompleted {
			m.logger.Info("query_done_via_pty_exit", "userID", userID)
		}

		// Retain for 5 minutes, then clean up.
		time.AfterFunc(userSessionRetain, func() {
			m.mu.Lock()
			cur, ok := m.sessions[userID]
			if ok && cur == sess {
				delete(m.sessions, userID)
			}
			m.mu.Unlock()
			sess.closeAllJSON()
		})
	}()

	// Watch for timeout — kill PTY if still running.
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			m.logger.Warn("UserSession timeout, stopping PTY", "userID", userID)
			sess.pty.stop()
		}
	}()

	return nil
}

// ClaimUUID associates a session_uuid with a user session.
// Called by the hook handler when the first hook event arrives for a new session.
// SubscribeJSON returns a JSON event channel for the given userID session.
func (m *UserSessionManager) SubscribeJSON(userID string) (<-chan string, func(), bool) {
	m.mu.Lock()
	sess, ok := m.sessions[userID]
	m.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	ch, unsub := sess.subscribeJSON()
	return ch, unsub, true
}

// ListSessions implements SessionProvider.
func (m *UserSessionManager) ListSessions() []SessionView {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SessionView, 0, len(m.sessions))
	for _, sess := range m.sessions {
		out = append(out, SessionView{
			ChannelID: sess.userID,
		})
	}
	return out
}

// SubscribeSession returns a PTY output channel for the given userID.
func (m *UserSessionManager) SubscribeSession(userID string) (<-chan []byte, func(), bool) {
	m.mu.Lock()
	sess, ok := m.sessions[userID]
	m.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	ch, unsub := sess.pty.subscribe()
	return ch, unsub, true
}

// ResizeSession resizes the PTY for the given userID.
func (m *UserSessionManager) ResizeSession(userID string, cols, rows uint16) {
	m.mu.Lock()
	sess, ok := m.sessions[userID]
	m.mu.Unlock()
	if ok {
		sess.pty.resize(cols, rows)
	}
}

// WriteSession writes data to the PTY for the given userID.
func (m *UserSessionManager) WriteSession(userID string, data []byte) error {
	m.mu.Lock()
	sess, ok := m.sessions[userID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session not found: %s", userID)
	}
	return sess.pty.write(data)
}

// sessionConflictError indicates a session is already running for the user.
type sessionConflictError struct{ userID string }

func (e *sessionConflictError) Error() string {
	return fmt.Sprintf("session already running for user %s", e.userID)
}

func (e *sessionConflictError) IsConflict() bool { return true }

