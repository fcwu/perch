package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// acpChatSession represents one active ACP-based chat-API session for a user.
type acpChatSession struct {
	userID         string
	username       string
	query          string // display form (with [image:filename] [file:filename] placeholders for management history)
	rawText        string // raw user text (without placeholders), used for prompt building
	attachments    []Attachment // images only; non-images are persisted to disk before runPrompt
	persistedFiles []PersistedAttachment // non-image attachments written under <workdir>/uploads/<convID>/
	conversationID string
	sessionID      string // store session UUID (set after InsertSession)
	poolKey        string

	mu     sync.Mutex
	jsonCh chan string
	done   bool
}

func newACPChatSession(userID, username, query, conversationID string, images []Attachment, persisted []PersistedAttachment) *acpChatSession {
	convID := conversationID
	if convID == "" {
		convID = "default"
	}
	return &acpChatSession{
		userID:         userID,
		username:       username,
		query:          formatQueryForHistory(query, images, persisted),
		rawText:        query,
		attachments:    images,
		persistedFiles: persisted,
		conversationID: conversationID,
		sessionID:      newUUID(),
		poolKey:        "chat-api:" + userID + ":" + convID,
		jsonCh:         make(chan string, 256),
	}
}

// formatQueryForHistory builds the placeholder-prefixed string stored in
// query_sessions.query so the management history list does not embed base64
// data or absolute paths. Format: "[image:foo.png] [file:bar.pdf] <text>".
func formatQueryForHistory(text string, images []Attachment, persisted []PersistedAttachment) string {
	if len(images) == 0 && len(persisted) == 0 {
		return text
	}
	var b strings.Builder
	for _, a := range images {
		b.WriteString("[image:")
		b.WriteString(a.Filename)
		b.WriteString("] ")
	}
	for _, p := range persisted {
		b.WriteString("[file:")
		b.WriteString(p.Filename)
		b.WriteString("] ")
	}
	b.WriteString(text)
	return b.String()
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

// ChatSessionManager is the interface used by the chat handlers in server.go.
type ChatSessionManager interface {
	StartSession(userID, username, query string, newConversation bool, conversationID string, attachments []Attachment) error
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
	workdir  string
	settings *SettingsManager // optional; nil → built-in defaults
	imgStore *imageStore

	defaultRuntime AgentRuntime
	runtimeFor     func(name string) (AgentRuntime, error)
	mcpServersFor  func(rt AgentRuntime, userID, conversationID string) []map[string]any

	mu       sync.Mutex
	sessions map[string]*acpChatSession // userID → active session
}

func newACPUserSessionManager(runtime AgentRuntime, workdir string, store *Store, adminHub *ManagementHub, logger *slog.Logger) *ACPUserSessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	pool := newACPSessionPool(runtime.ACPExecutable, runtime.ACPArgs, workdir, logger)
	m := &ACPUserSessionManager{
		pool:           pool,
		store:          store,
		adminHub:       adminHub,
		logger:         logger,
		timeout:        acpRunTimeout(),
		workdir:        workdir,
		defaultRuntime: runtime,
		runtimeFor:     agentRuntimeByName,
		sessions:       make(map[string]*acpChatSession),
		imgStore:       newImageStore(workdir, logger),
	}
	// Wire per-key process construction so chat-api:<user>:<conv> spawns the
	// runtime + MCP servers configured for that conversation. Non-chat keys
	// fall back to the default runtime (the one passed at construction).
	pool.keyFactory = func(key string) *ACPProcess {
		userID, convID, ok := chatAPIPoolKeyParts(key)
		if !ok {
			return nil
		}
		rt := m.defaultRuntime
		if store != nil {
			conv, err := store.GetConversation(convID, userID)
			if err == nil && conv != nil && conv.Runtime != "" {
				if resolved, rerr := m.resolveRuntime(conv.Runtime); rerr == nil {
					rt = resolved
				}
			}
		}
		proc := NewACPProcess(rt.ACPExecutable, rt.ACPArgs, workdir, logger)
		// mcpServers payload: empty when the runtime doesn't honour MCP.
		if rt.SupportsMCP && m.mcpServersFor != nil {
			servers := m.mcpServersFor(rt, userID, convID)
			if len(servers) > 0 {
				asAny := make([]any, 0, len(servers))
				for _, s := range servers {
					asAny = append(asAny, s)
				}
				proc.SetMCPServers(asAny)
			}
		}
		return proc
	}
	// Wire eviction hook so per-conversation uploads are cleaned up when the
	// pool drops the (user, conv) subprocess. The pool key for chat-api is
	// "chat-api:<userID>:<convID>"; we extract <convID> and rm -rf its dir.
	pool.evictHook = func(key string) {
		convID, ok := chatAPIConvIDFromPoolKey(key)
		if !ok || convID == "default" {
			return
		}
		convDir := filepath.Join(workdir, "uploads", convID)
		if err := os.RemoveAll(convDir); err != nil {
			logger.Warn("ACP chat: cleanup uploads dir failed", "convID", convID, "err", err)
		} else {
			logger.Info("ACP chat: cleaned uploads dir on evict", "convID", convID)
		}
		m.imgStore.Cleanup(convID)
	}
	return m
}

// SetSettings wires the settings manager so the chat API can read attachment
// limits at request time.
func (m *ACPUserSessionManager) SetSettings(sm *SettingsManager) {
	m.settings = sm
}

// Pool exposes the underlying ACP session pool so external callers (e.g. the
// HTTP server) can evict entries when the conversation's runtime/model change.
func (m *ACPUserSessionManager) Pool() *ACPSessionPool {
	return m.pool
}

// SetRuntimeResolver wires a resolver that returns the AgentRuntime for a
// given runtime id. The chat manager uses it to spawn the right ACP binary
// based on the conversation's runtime column.
func (m *ACPUserSessionManager) SetRuntimeResolver(fn func(name string) (AgentRuntime, error)) {
	m.runtimeFor = fn
}

// SetMCPServers wires the perch self-hosted MCP server descriptor builder.
// Called per session/new to populate the mcpServers field. Returning a nil or
// empty slice means the runtime gets `mcpServers: []`.
func (m *ACPUserSessionManager) SetMCPServers(fn func(rt AgentRuntime, userID, conversationID string) []map[string]any) {
	m.mcpServersFor = fn
}

// RunScheduledPrompt is the dispatcher entry point for chat_schedules fires.
// It runs prompt against the (user, conv) ACP session under
// `source='schedule'` semantics, persists the turn, and returns when the ACP
// run is complete (or errors out).
func (m *ACPUserSessionManager) RunScheduledPrompt(sch ChatSchedule) error {
	if m.store == nil {
		return fmt.Errorf("chat manager: store is nil")
	}
	conv, err := m.store.GetConversation(sch.ConversationID, sch.UserID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation not found")
	}

	// Skip-and-log if the conversation's runtime is no longer registered.
	if m.runtimeFor != nil && conv.Runtime != "" {
		if _, rerr := m.runtimeFor(conv.Runtime); rerr != nil {
			return fmt.Errorf("runtime not registered: %w", rerr)
		}
	}

	username := conv.UserID // best-effort; chat IM uses the userID-as-username convention
	sessID := newUUID()
	if err := m.store.InsertSessionWithSource(sessID, conv.UserID, username, sch.Prompt, conv.ID, "schedule"); err != nil {
		return fmt.Errorf("insert query_session: %w", err)
	}

	poolKey := "chat-api:" + conv.UserID + ":" + conv.ID
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	proc, err := m.pool.Acquire(ctx, poolKey)
	if err != nil {
		_ = m.store.UpdateSessionError(sessID, err.Error())
		return fmt.Errorf("acquire pool session: %w", err)
	}
	defer m.pool.Release(poolKey)

	blocks := []ACPContent{{Type: "text", Text: sch.Prompt}}
	response, err := proc.PromptWithContent(ctx, blocks, nil, nil, nil)
	if err != nil {
		_ = m.store.UpdateSessionError(sessID, err.Error())
		return fmt.Errorf("acp prompt: %w", err)
	}
	if err := m.store.UpdateSessionDoneAndTouch(sessID, response, conv.ID); err != nil {
		m.logger.Warn("RunScheduledPrompt: mark done failed", "err", err)
	}
	return nil
}

// chatAPIConvIDFromPoolKey extracts the conversation ID from a pool key of
// the form "chat-api:<userID>:<convID>". Returns ok=false for non-chat keys.
func chatAPIConvIDFromPoolKey(key string) (string, bool) {
	_, conv, ok := chatAPIPoolKeyParts(key)
	return conv, ok
}

// chatAPIPoolKeyParts splits a chat-api pool key into (userID, convID).
func chatAPIPoolKeyParts(key string) (string, string, bool) {
	const prefix = "chat-api:"
	if !strings.HasPrefix(key, prefix) {
		return "", "", false
	}
	rest := key[len(prefix):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return "", "", false
	}
	return rest[:colon], rest[colon+1:], true
}

// resolveRuntime delegates to the wired resolver, falling back to the
// canonical registry. Used by the per-key process factory and the scheduled
// fire path so an unknown runtime fails closed.
func (m *ACPUserSessionManager) resolveRuntime(name string) (AgentRuntime, error) {
	if m.runtimeFor != nil {
		return m.runtimeFor(name)
	}
	return agentRuntimeByName(name)
}

// StartSession starts an ACP prompt for userID. Returns 409-alike error if busy.
func (m *ACPUserSessionManager) StartSession(userID, username, query string, newConversation bool, conversationID string, attachments []Attachment) error {
	// Partition attachments: images stay in memory (inline ACP image block);
	// non-images are persisted under <workdir>/uploads/<convID>/.
	var images []Attachment
	var nonImages []Attachment
	for _, a := range attachments {
		if IsImageMIME(a.MimeType) {
			images = append(images, a)
		} else {
			nonImages = append(nonImages, a)
		}
	}

	convForDir := conversationID
	if convForDir == "" {
		convForDir = "default"
	}

	var persisted []PersistedAttachment
	if len(nonImages) > 0 {
		var cs *ChatSettings
		if m.settings != nil {
			cs = m.settings.GetEffective().Chat
		}
		lim := EffectiveAttachmentLimits(cs)
		// Persist with a short timeout — disk writes should be quick.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		persisted, err = WriteAttachmentsToDisk(ctx, m.workdir, convForDir, nonImages, lim)
		if err != nil {
			// Preserve the sentinel so the HTTP handler can map quota errors
			// to 400 Bad Request instead of a generic 500.
			return fmt.Errorf("persist attachments: %w", err)
		}
	}

	m.mu.Lock()
	existing, ok := m.sessions[userID]
	if ok {
		existing.mu.Lock()
		running := !existing.done
		existing.mu.Unlock()
		if running {
			m.mu.Unlock()
			// Roll back disk writes on conflict — caller's request is rejected.
			for _, p := range persisted {
				_ = os.Remove(p.AbsPath)
			}
			return &sessionConflictError{userID: userID}
		}
		delete(m.sessions, userID)
	}

	sess := newACPChatSession(userID, username, query, conversationID, images, persisted)
	m.sessions[userID] = sess
	m.mu.Unlock()

	// Build text prompt: history expansion (if continuing) + file prefix
	// (when non-image attachments were persisted) + the user's query.
	promptText := BuildPromptFilePrefix(persisted, query)
	if !newConversation && m.store != nil {
		since := time.Now().UnixMilli() - conversationWindow.Milliseconds()
		if history, err := m.store.GetRecentHistory(userID, since, conversationMaxTurns); err != nil {
			m.logger.Warn("ACP chat: GetRecentHistory failed", "err", err)
		} else {
			promptText = buildPrompt(history, BuildPromptFilePrefix(persisted, query))
		}
	}

	if len(attachments) > 0 {
		var imgBytes int
		for _, a := range images {
			imgBytes += len(a.DataBase64)
		}
		var fileBytes int64
		for _, p := range persisted {
			fileBytes += p.SizeBytes
		}
		m.logger.Info("ACP chat: attachments accepted",
			"userID", userID,
			"images", len(images),
			"image_b64_bytes", imgBytes,
			"files", len(persisted),
			"file_bytes", fileBytes,
		)
	}
	go m.runPrompt(sess, promptText)
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
			// Lazy-backfill runtime/model on legacy rows so the per-key
			// process factory has a deterministic runtime to work with.
			if conv, err := m.store.GetConversation(sess.conversationID, sess.userID); err == nil && conv != nil {
				if conv.Runtime == "" {
					rt := m.defaultRuntime
					if err := m.store.BackfillConversationRuntime(conv.ID, rt.Name, rt.DefaultModel); err != nil {
						m.logger.Warn("ACP chat: backfill runtime failed", "convID", conv.ID, "err", err)
					}
				}
			}
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
		if m.store != nil {
			_ = m.store.UpdateSessionError(sess.sessionID, err.Error())
		}
		return
	}
	defer m.pool.Release(sess.poolKey)

	onChunk := func(chunk string) {
		msg, _ := json.Marshal(map[string]string{"type": "chunk", "text": chunk})
		sess.broadcastJSON(string(msg))
	}
	var currentToolEventID int64
	var currentToolName string
	var currentToolStart time.Time
	onToolStart := func(toolName, inputJSON string) {
		currentToolName = toolName
		currentToolStart = time.Now()
		if m.adminHub != nil {
			m.adminHub.SessionUpdated(sess.sessionID, toolName)
		}
		if m.store != nil {
			if id, err := m.store.InsertToolEvent(sess.sessionID, toolName, inputJSON); err == nil {
				currentToolEventID = id
			} else {
				m.logger.Warn("ACP chat: insert tool event", "err", err)
			}
		}
		toolMsg, _ := json.Marshal(map[string]string{"type": "tool_start", "tool": toolName})
		sess.broadcastJSON(string(toolMsg))
	}
	onToolEnd := func(outputJSON string) {
		elapsedMs := time.Since(currentToolStart).Milliseconds()
		toolEndMsg, _ := json.Marshal(map[string]interface{}{"type": "tool_end", "tool": currentToolName, "elapsed_ms": elapsedMs})
		sess.broadcastJSON(string(toolEndMsg))
		if m.adminHub != nil {
			m.adminHub.SessionUpdated(sess.sessionID, "")
		}
		if m.store != nil && currentToolEventID > 0 {
			if err := m.store.UpdateToolEventEnd(currentToolEventID, outputJSON); err != nil {
				m.logger.Warn("ACP chat: update tool event", "err", err)
			}
			currentToolEventID = 0
		}
		currentToolName = ""
	}

	// sess.attachments holds only image attachments (non-images were persisted
	// to disk and are referenced via the `[file: ...]` prefix already baked
	// into prompt by BuildPromptFilePrefix).
	blocks := []ACPContent{{Type: "text", Text: prompt}}
	blocks = append(blocks, AttachmentsToACPBlocks(sess.attachments)...)
	response, err := proc.PromptWithContent(ctx, blocks, onChunk, onToolStart, onToolEnd)

	// Extract [image: ...] tokens and store images; cleanText has tokens removed.
	convID := sess.conversationID
	if convID == "" {
		convID = "default"
	}
	var imgAttachments []ImageAttachment
	cleanText := response
	if err == nil {
		cleanText, imgAttachments = extractImages(response, m.imgStore, m.workdir, convID)
	}

	if err != nil {
		errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		sess.broadcastJSON(string(errMsg))
		if m.store != nil {
			_ = m.store.UpdateSessionError(sess.sessionID, err.Error())
		}
		if m.adminHub != nil {
			m.adminHub.SessionRemoved(sess.sessionID, "error")
		}
	} else {
		if m.store != nil {
			if sess.conversationID != "" {
				_ = m.store.UpdateSessionDoneAndTouch(sess.sessionID, cleanText, sess.conversationID)
			} else {
				_ = m.store.UpdateSessionDone(sess.sessionID, cleanText)
			}
		}
		if m.adminHub != nil {
			m.adminHub.SessionRemoved(sess.sessionID, "done")
		}
	}

	if imgAttachments == nil {
		imgAttachments = []ImageAttachment{}
	}
	doneMsg, _ := json.Marshal(map[string]any{"type": "done", "images": imgAttachments, "text": cleanText})
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
