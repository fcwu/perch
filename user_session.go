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
	userID      string
	username    string
	sessionUUID string
	query       string  // original user query (stored in DB, shown in admin)
	prompt      string  // final injected prompt sent to the agent (may include history)
	startedAt   int64

	pty    *PTYManager
	status userSessionStatus

	mu        sync.Mutex
	jsonSubs  map[int]chan string
	nextSubID int

	toolStartTimes  map[string]time.Time  // toolName → PreToolUse time
	toolEventIDs    map[string]int64      // toolName → store event ID

	cancel      context.CancelFunc
	completedAt time.Time
}

func newUserSession(userID, username, query string) *userSession {
	return &userSession{
		userID:       userID,
		username:     username,
		query:        query,
		startedAt:    time.Now().UnixMilli(),
		pty:          newPTYManager(),
		jsonSubs:     make(map[int]chan string),
		toolStartTimes: make(map[string]time.Time),
		toolEventIDs:   make(map[string]int64),
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
	adminHub  *AdminHub

	mu       sync.Mutex
	sessions map[string]*userSession // userID → session
	uuidMap  map[string]string       // sessionUUID → userID (for hook routing)
}

func newUserSessionManager(runtime AgentRuntime, workdir string, logger *slog.Logger, store *Store, adminHub *AdminHub) *UserSessionManager {
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
		uuidMap:  make(map[string]string),
	}
}

// StartSession creates a new OpenCode session for the user.
// When newConversation is false, recent history is prepended to the query.
// Returns error with HTTP 409 status hint if a session is already running.
func (m *UserSessionManager) StartSession(userID, username, query string, newConversation bool) error {
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
		if existing.sessionUUID != "" {
			delete(m.uuidMap, existing.sessionUUID)
		}
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
	m.sessions[userID] = sess
	m.mu.Unlock()

	cmd, args := m.runtime.RunAgent("as-query", prompt, m.workdir)

	ctx, cancel := context.WithTimeout(context.Background(), userSessionTimeout)
	sess.cancel = cancel

	go func() {
		defer cancel()
		// startOnce runs the PTY once and closes p.done on exit.
		sess.pty.startOnce(cmd, args, m.workdir, m.logger)

		// PTY exited (EOF) — mark completed.
		sess.mu.Lock()
		sess.status = userSessionCompleted
		sess.completedAt = time.Now()
		sess.mu.Unlock()

		m.logger.Info("UserSession PTY exited", "userID", userID)

		// Retain for 5 minutes, then clean up.
		time.AfterFunc(userSessionRetain, func() {
			m.mu.Lock()
			cur, ok := m.sessions[userID]
			if ok && cur == sess {
				if sess.sessionUUID != "" {
					delete(m.uuidMap, sess.sessionUUID)
				}
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
			durationMs := time.Now().UnixMilli() - sess.startedAt
			if m.store != nil && sess.sessionUUID != "" {
				if err := m.store.UpdateSessionTimeout(sess.sessionUUID); err != nil {
					m.logger.Error("store: update session timeout", "err", err)
				}
			}
			if m.adminHub != nil && sess.sessionUUID != "" {
				m.adminHub.SessionRemoved(sess.sessionUUID, "timeout")
			}
			m.logger.Info("query_done", "session_id", sess.sessionUUID, "duration_ms", durationMs, "status", "timeout")
			sess.pty.stop()
		}
	}()

	return nil
}

// ClaimUUID associates a session_uuid with a user session.
// Called by the hook handler when the first hook event arrives for a new session.
// If uuid is already known, returns the userID. If not, tries to find a pending session.
func (m *UserSessionManager) ClaimUUID(uuid string) (*userSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if userID, ok := m.uuidMap[uuid]; ok {
		if sess, ok := m.sessions[userID]; ok {
			return sess, true
		}
		return nil, false
	}

	// Find a running session that hasn't claimed a UUID yet.
	for _, sess := range m.sessions {
		if sess.sessionUUID == "" {
			sess.mu.Lock()
			st := sess.status
			sess.mu.Unlock()
			if st == userSessionRunning {
				sess.sessionUUID = uuid
				m.uuidMap[uuid] = sess.userID
				// Persist session start now that we have the UUID.
				if m.store != nil {
					if err := m.store.InsertSession(uuid, sess.userID, sess.username, sess.query); err != nil {
						m.logger.Error("store: insert session", "err", err)
					}
				}
				if m.adminHub != nil {
					m.adminHub.SessionAdded(AdminSessionView{
						ID:        uuid,
						Username:  sess.username,
						Query:     sess.query,
						Status:    "running",
						StartedAt: sess.startedAt,
					})
				}
				m.logger.Info("query_start", "user_id", sess.userID, "username", sess.username, "session_id", uuid, "query", sess.query)
				return sess, true
			}
		}
	}
	return nil, false
}

// SessionByUUID returns the session for the given uuid, or nil.
func (m *UserSessionManager) SessionByUUID(uuid string) (*userSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	userID, ok := m.uuidMap[uuid]
	if !ok {
		return nil, false
	}
	sess, ok := m.sessions[userID]
	return sess, ok
}

// NotifyHook processes a hook event and routes it to the appropriate user session.
func (m *UserSessionManager) NotifyHook(event HookEvent) {
	var sess *userSession
	var ok bool
	if event.SessionID != "" {
		sess, ok = m.SessionByUUID(event.SessionID)
		if !ok {
			sess, ok = m.ClaimUUID(event.SessionID)
		}
	}
	if !ok || sess == nil {
		m.logger.Debug("UserSessionManager: no session for hook event", "sessionID", event.SessionID, "event", event.EventName)
		return
	}

	switch event.EventName {
	case "PreToolUse":
		sess.mu.Lock()
		sess.toolStartTimes[event.ToolName] = time.Now()
		sess.mu.Unlock()

		input := truncateJSON(event.ToolInput, 200)
		msg, _ := json.Marshal(map[string]any{
			"type":  "tool_start",
			"tool":  event.ToolName,
			"input": json.RawMessage(input),
		})
		sess.broadcastJSON(string(msg))

		// Persist tool event start
		if m.store != nil && sess.sessionUUID != "" {
			if eventID, err := m.store.InsertToolEvent(sess.sessionUUID, event.ToolName, string(input)); err != nil {
				m.logger.Error("store: insert tool event", "err", err)
			} else {
				sess.mu.Lock()
				sess.toolEventIDs[event.ToolName] = eventID
				sess.mu.Unlock()
			}
		}
		if m.adminHub != nil && sess.sessionUUID != "" {
			m.adminHub.SessionUpdated(sess.sessionUUID, event.ToolName)
		}
		m.logger.Info("tool_start", "session_id", sess.sessionUUID, "tool", event.ToolName)

	case "PostToolUse":
		sess.mu.Lock()
		start, had := sess.toolStartTimes[event.ToolName]
		if had {
			delete(sess.toolStartTimes, event.ToolName)
		}
		eventID, hasEventID := sess.toolEventIDs[event.ToolName]
		if hasEventID {
			delete(sess.toolEventIDs, event.ToolName)
		}
		sess.mu.Unlock()

		var elapsed int64
		if had {
			elapsed = time.Since(start).Milliseconds()
		}
		msg, _ := json.Marshal(map[string]any{
			"type":       "tool_end",
			"tool":       event.ToolName,
			"elapsed_ms": elapsed,
		})
		sess.broadcastJSON(string(msg))

		// Persist tool event end
		if m.store != nil && hasEventID {
			outputJSON := string(truncateJSON(event.ToolResponse, 2000))
			if err := m.store.UpdateToolEventEnd(eventID, outputJSON); err != nil {
				m.logger.Error("store: update tool event end", "err", err)
			}
		}
		if m.adminHub != nil && sess.sessionUUID != "" {
			m.adminHub.SessionUpdated(sess.sessionUUID, "")
		}

	case "Stop":
		msg, _ := json.Marshal(map[string]string{"type": "done"})
		sess.broadcastJSON(string(msg))

		sess.mu.Lock()
		sess.status = userSessionCompleted
		sess.completedAt = time.Now()
		sess.mu.Unlock()
		if sess.cancel != nil {
			sess.cancel()
		}

		// Persist session completion
		durationMs := time.Now().UnixMilli() - sess.startedAt
		sess.mu.Lock()
		toolCount := len(sess.toolStartTimes)
		sess.mu.Unlock()
		if m.store != nil && sess.sessionUUID != "" {
			if err := m.store.UpdateSessionDone(sess.sessionUUID, event.LastAssistantMessage); err != nil {
				m.logger.Error("store: update session done", "err", err)
			}
		}
		if m.adminHub != nil && sess.sessionUUID != "" {
			m.adminHub.SessionRemoved(sess.sessionUUID, "done")
		}
		m.logger.Info("query_done", "session_id", sess.sessionUUID, "duration_ms", durationMs, "tool_count", toolCount, "status", "done")
	}
}

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
			ChannelID:   sess.userID,
			SessionUUID: sess.sessionUUID,
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

// truncateJSON truncates a raw JSON value to at most maxBytes, returning valid JSON.
func truncateJSON(raw json.RawMessage, maxBytes int) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	if len(raw) <= maxBytes {
		return raw
	}
	// Return a JSON string noting the truncation.
	truncated, _ := json.Marshal(fmt.Sprintf("<truncated %d bytes>", len(raw)))
	return truncated
}
