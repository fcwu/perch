package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// --- Mock Discord HTTP transport ---

type discordOp struct {
	Method  string
	Emoji   string
	Content string
}

type mockDiscordRT struct {
	mu  sync.Mutex
	ops []discordOp
}

func (m *mockDiscordRT) RoundTrip(r *http.Request) (*http.Response, error) {
	path := r.URL.Path

	switch {
	case strings.Contains(path, "/reactions/") && r.Method == http.MethodPut:
		parts := strings.SplitN(path, "/reactions/", 2)
		emoji := strings.TrimSuffix(parts[1], "/@me")
		m.mu.Lock()
		m.ops = append(m.ops, discordOp{Method: "ADD_REACTION", Emoji: emoji})
		m.mu.Unlock()
		return &http.Response{StatusCode: 204, Body: http.NoBody, Header: make(http.Header)}, nil

	case strings.Contains(path, "/reactions/") && r.Method == http.MethodDelete:
		parts := strings.SplitN(path, "/reactions/", 2)
		emoji := strings.TrimSuffix(parts[1], "/@me")
		m.mu.Lock()
		m.ops = append(m.ops, discordOp{Method: "REMOVE_REACTION", Emoji: emoji})
		m.mu.Unlock()
		return &http.Response{StatusCode: 204, Body: http.NoBody, Header: make(http.Header)}, nil

	case strings.HasSuffix(path, "/messages") && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		content, _ := body["content"].(string)
		m.mu.Lock()
		m.ops = append(m.ops, discordOp{Method: "SEND_MESSAGE", Content: content})
		m.mu.Unlock()
		fake := `{"id":"99999","channel_id":"test-ch","content":"` + content + `"}`
		hdr := http.Header{}
		hdr.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(fake)),
			Header:     hdr,
		}, nil
	}
	return &http.Response{StatusCode: 204, Body: http.NoBody, Header: make(http.Header)}, nil
}

func (m *mockDiscordRT) reactions(kind string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, op := range m.ops {
		if op.Method == kind {
			out = append(out, op.Emoji)
		}
	}
	return out
}

func (m *mockDiscordRT) messages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, op := range m.ops {
		if op.Method == "SEND_MESSAGE" {
			out = append(out, op.Content)
		}
	}
	return out
}

func newMockDiscordSession(rt http.RoundTripper) *discordgo.Session {
	s, _ := discordgo.New("Bot testtoken")
	s.Client = &http.Client{Transport: rt}
	return s
}

// --- Helpers ---

// newTestDiscordManager builds a DiscordSessionManager without a live connection.
func newTestDiscordManager(_ bool) *DiscordSessionManager {
	return &DiscordSessionManager{
		runtime:        AgentRuntime{},
		sessions:       make(map[string]*discordSession),
		channelPrivate: make(map[string]bool),
		logger:         slog.Default(),
	}
}

// newFakeACPProcess returns an *ACPProcess backed by in-process pipes and its server helper.
// The process starts with running=true and the given sessionID already set.
// The fakeACPServer (from acp_process_test.go) drives protocol responses.
func newFakeACPProcess(t *testing.T, sessionID string) (*ACPProcess, *fakeACPServer) {
	t.Helper()
	proc, srv := newFakeACPServer(t)
	proc.running = true
	proc.sessionID = sessionID
	return proc, srv
}

// newACPSession creates a discordSession in ACP mode with a pre-wired fake process.
func newACPSession(t *testing.T, channelID, sessionID string) (*discordSession, *fakeACPServer) {
	t.Helper()
	proc, srv := newFakeACPProcess(t, sessionID)
	sess := &discordSession{
		channelID:  channelID,
		runtime:    AgentRuntime{},
		acpProcess: proc,
	}
	return sess, srv
}

// servePromptSuccess responds to one prompt call with chunks and a completion response.
func servePromptSuccess(t *testing.T, srv *fakeACPServer, sessionID, text string) {
	t.Helper()
	go func() {
		req := srv.readRequest(t)
		if req.Method != "session/prompt" {
			t.Errorf("expected session/prompt, got %q", req.Method)
		}
		srv.sendNotification("session/update", map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": text},
			},
		})
		srv.sendResponse(rawIDToInt64(req.ID), map[string]any{"status": "completed"})
	}()
}

// servePromptError responds to one prompt call with an RPC error.
func servePromptError(t *testing.T, srv *fakeACPServer, errMsg string) {
	t.Helper()
	go func() {
		req := srv.readRequest(t)
		srv.sendError(rawIDToInt64(req.ID), errMsg)
	}()
}

// ============================================================
// T70 — acpRunTimeout env var parsing
// ============================================================

func TestACPRunTimeout(t *testing.T) {
	t.Cleanup(func() { os.Unsetenv("ACP_RUN_TIMEOUT") })

	os.Unsetenv("ACP_RUN_TIMEOUT")
	if got := acpRunTimeout(); got != 5*time.Minute {
		t.Errorf("no env: got %v, want 5m", got)
	}

	os.Setenv("ACP_RUN_TIMEOUT", "30")
	if got := acpRunTimeout(); got != 30*time.Second {
		t.Errorf("30s: got %v, want 30s", got)
	}

	os.Setenv("ACP_RUN_TIMEOUT", "abc")
	if got := acpRunTimeout(); got != 5*time.Minute {
		t.Errorf("invalid fallback: got %v, want 5m", got)
	}

	os.Setenv("ACP_RUN_TIMEOUT", "0")
	if got := acpRunTimeout(); got != 5*time.Minute {
		t.Errorf("zero fallback: got %v, want 5m", got)
	}
}

// ============================================================
// T64 — newDiscordSession always uses ACP mode
// ============================================================

func TestDiscordACPMode(t *testing.T) {
	sess := newDiscordSession(AgentRuntime{ACPExecutable: "fake-agent-acp"}, "ch-acp", "", nil, slog.Default())
	if sess.acpProcess == nil {
		t.Error("ACP mode: acpProcess should be non-nil")
	}
}

// ============================================================
// T72 — SubscribeSession returns false for ACP session
// ============================================================

func TestDiscordACPSubscribeSession(t *testing.T) {
	mgr := newTestDiscordManager(true)
	proc, srv := newFakeACPProcess(t, "sess-sub")
	defer srv.close()
	mgr.sessions["test-ch"] = &discordSession{channelID: "test-ch", acpProcess: proc}

	ch, unsub, ok := mgr.SubscribeSession("test-ch")
	if ok {
		t.Error("ACP session: SubscribeSession should return false")
	}
	if ch != nil || unsub != nil {
		t.Error("ACP session: SubscribeSession should return nil ch/unsub")
	}
}

// ============================================================
// T73 — WriteSession returns error for ACP session
// ============================================================

func TestDiscordACPWriteSession(t *testing.T) {
	mgr := newTestDiscordManager(true)
	proc, srv := newFakeACPProcess(t, "sess-write")
	defer srv.close()
	mgr.sessions["test-ch"] = &discordSession{channelID: "test-ch", acpProcess: proc}

	err := mgr.WriteSession("test-ch", []byte("hello"))
	if err == nil {
		t.Error("ACP session: WriteSession should return error")
	}
	if !strings.Contains(err.Error(), "ACP") {
		t.Errorf("error should mention ACP, got: %v", err)
	}
}

// T71 was: Notify skips ACP sessions — removed (Notify no longer exists; Discord is ACP-only)

// ============================================================
// T66 — Happy path: 👀 → EnsureRunning → Prompt → 💬 + reply
// ============================================================

func TestDiscordACPHappyPath(t *testing.T) {
	sess, srv := newACPSession(t, "ch-ok", "sess-ok")
	defer srv.close()

	servePromptSuccess(t, srv, "sess-ok", "Hello!")

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-ok", "msg-001", "ping", nil, nil, nil, nil, slog.Default())

	added := rt.reactions("ADD_REACTION")
	removed := rt.reactions("REMOVE_REACTION")
	msgs := rt.messages()

	if len(added) < 2 {
		t.Fatalf("expected >=2 reactions added (👀, 💬), got %v", added)
	}
	if added[0] != emojiEyes {
		t.Errorf("first reaction should be 👀, got %q", added[0])
	}
	hasEyesRemoved := false
	for _, e := range removed {
		if e == emojiEyes {
			hasEyesRemoved = true
		}
	}
	if !hasEyesRemoved {
		t.Error("👀 should be removed after completion")
	}
	hasSpeech := false
	for _, e := range added {
		if e == emojiSpeech {
			hasSpeech = true
		}
	}
	if !hasSpeech {
		t.Error("💬 should be added on success")
	}
	if len(msgs) == 0 || msgs[0] != "Hello!" {
		t.Errorf("expected reply 'Hello!', got %v", msgs)
	}
}

// ============================================================
// T67 — ACP process fails to start: Discord receives ❌ + error
// ============================================================

func TestDiscordACPServerUnreachable(t *testing.T) {
	os.Setenv("ACP_RUN_TIMEOUT", "3")
	defer os.Unsetenv("ACP_RUN_TIMEOUT")

	// Session with a process that is not running (needs to EnsureRunning) but
	// will fail because the executable doesn't exist.
	proc := NewACPProcess("/nonexistent-binary-xyz", nil, t.TempDir(), slog.Default())
	sess := &discordSession{channelID: "ch-err", acpProcess: proc}

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-err", "msg-002", "ping", nil, nil, nil, nil, slog.Default())

	added := rt.reactions("ADD_REACTION")
	removed := rt.reactions("REMOVE_REACTION")
	msgs := rt.messages()

	if len(added) == 0 || added[0] != emojiEyes {
		t.Errorf("first reaction should be 👀, got %v", added)
	}
	hasEyesRemoved := false
	for _, e := range removed {
		if e == emojiEyes {
			hasEyesRemoved = true
		}
	}
	if !hasEyesRemoved {
		t.Error("👀 should be removed after error")
	}
	hasCross := false
	for _, e := range added {
		if e == emojiCross {
			hasCross = true
		}
	}
	if !hasCross {
		t.Error("❌ should be added on error")
	}
	if len(msgs) == 0 {
		t.Error("error message should be sent to Discord")
	}
	if !strings.HasPrefix(msgs[0], "❌ Agent unavailable:") {
		t.Errorf("expected '❌ Agent unavailable:...', got %q", msgs[0])
	}
}

// ============================================================
// T68 — Prompt returns error: Discord receives ❌
// ============================================================

func TestDiscordACPServerError(t *testing.T) {
	sess, srv := newACPSession(t, "ch-5xx", "sess-5xx")
	defer srv.close()

	servePromptError(t, srv, "agent internal error 500")

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-5xx", "msg-003", "ping", nil, nil, nil, nil, slog.Default())

	msgs := rt.messages()
	if len(msgs) == 0 {
		t.Fatal("expected error reply to Discord")
	}
	if !strings.HasPrefix(msgs[0], "❌") {
		t.Errorf("expected ❌ prefix, got %q", msgs[0])
	}
}

// ============================================================
// T74 — Prompt error with message: Discord receives ❌ with text
// ============================================================

func TestDiscordACPRunFailed(t *testing.T) {
	sess, srv := newACPSession(t, "ch-fail", "sess-fail")
	defer srv.close()

	servePromptError(t, srv, "agent internal error")

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-fail", "msg-004", "ping", nil, nil, nil, nil, slog.Default())

	added := rt.reactions("ADD_REACTION")
	hasCross, hasSpeech := false, false
	for _, e := range added {
		if e == emojiCross {
			hasCross = true
		}
		if e == emojiSpeech {
			hasSpeech = true
		}
	}
	if !hasCross {
		t.Error("❌ reaction should be added on RunFailed")
	}
	if hasSpeech {
		t.Error("💬 should NOT be added on error")
	}
	msgs := rt.messages()
	if len(msgs) == 0 {
		t.Fatal("error reply should be sent")
	}
	if !strings.Contains(msgs[0], "agent internal error") {
		t.Errorf("reply should contain error text, got %q", msgs[0])
	}
}

// ============================================================
// T69 — Run timeout: Discord receives ❌ + "⏱️ Agent timed out."
// ============================================================

func TestDiscordACPRunTimeout(t *testing.T) {
	os.Setenv("ACP_RUN_TIMEOUT", "1")
	defer os.Unsetenv("ACP_RUN_TIMEOUT")

	sess, srv := newACPSession(t, "ch-timeout", "sess-timeout")
	defer srv.close()

	// Server reads the prompt but never responds (simulates hang).
	go func() { srv.readRequest(t) }()

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)

	start := time.Now()
	sess.handleWithACP(dgo, "ch-timeout", "msg-005", "ping", nil, nil, nil, nil, slog.Default())
	if time.Since(start) > 5*time.Second {
		t.Errorf("should have timed out in ~1s, took %v", time.Since(start))
	}

	added := rt.reactions("ADD_REACTION")
	hasCross := false
	for _, e := range added {
		if e == emojiCross {
			hasCross = true
		}
	}
	if !hasCross {
		t.Error("❌ reaction should be added on timeout")
	}
	msgs := rt.messages()
	if len(msgs) == 0 {
		t.Fatal("timeout message should be sent")
	}
	if !strings.Contains(msgs[0], "⏱️ Agent timed out.") {
		t.Errorf("expected timeout message, got %q", msgs[0])
	}
	if strings.Contains(msgs[0], "context deadline exceeded") {
		t.Errorf("should not expose Go internal errors, got %q", msgs[0])
	}
}

// ============================================================
// T75 — Long reply is split into ≤1900-char chunks
// ============================================================

func TestDiscordACPLongReply(t *testing.T) {
	sess, srv := newACPSession(t, "ch-long", "sess-long")
	defer srv.close()

	longText := strings.Repeat(strings.Repeat("a", 100)+"\n", 20) // 2000+ chars
	servePromptSuccess(t, srv, "sess-long", longText)

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-long", "msg-006", "ping", nil, nil, nil, nil, slog.Default())

	msgs := rt.messages()
	if len(msgs) < 2 {
		t.Errorf("expected multiple chunks for 2000-char reply, got %d", len(msgs))
	}
	for i, m := range msgs {
		if len(m) > discordMaxLen {
			t.Errorf("chunk %d exceeds %d chars: len=%d", i, discordMaxLen, len(m))
		}
	}
}

// ============================================================
// T65 — DM allowlist is enforced in ACP mode
// ============================================================

func TestDiscordACPDMAllowlist(t *testing.T) {
	os.Setenv("ACP_RUN_TIMEOUT", "2")
	defer os.Unsetenv("ACP_RUN_TIMEOUT")

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	dgo.State.User = &discordgo.User{ID: "bot-001"}

	mgr := &DiscordSessionManager{
		runtime:          AgentRuntime{ACPExecutable: "/nonexistent-binary-xyz"}, // fails if actually invoked
		sessions:         make(map[string]*discordSession),
		channelPrivate:   make(map[string]bool),
		allowedDMUserIDs: map[string]struct{}{"111": {}},
		logger:           slog.Default(),
		dgo:              dgo,
	}

	// Unauthorized user "999" — no Discord activity expected.
	mgr.onMessage(dgo, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "msg-unauth",
		ChannelID: "dm-unauth",
		GuildID:   "",
		Author:    &discordgo.User{ID: "999", Bot: false},
		Content:   "hello",
	}})
	time.Sleep(200 * time.Millisecond)

	rt.mu.Lock()
	opsAfterUnauth := len(rt.ops)
	rt.mu.Unlock()
	if opsAfterUnauth != 0 {
		t.Errorf("unauthorized DM: expected no Discord activity, got %d ops", opsAfterUnauth)
	}

	// Authorized user "111" — expect ❌ (because binary doesn't exist), proving ACP was invoked.
	mgr.onMessage(dgo, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "msg-auth",
		ChannelID: "dm-auth",
		GuildID:   "",
		Author:    &discordgo.User{ID: "111", Bot: false},
		Content:   "hello",
	}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hasCross := false
		for _, e := range rt.reactions("ADD_REACTION") {
			if e == emojiCross {
				hasCross = true
			}
		}
		if hasCross {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	hasCross := false
	for _, e := range rt.reactions("ADD_REACTION") {
		if e == emojiCross {
			hasCross = true
		}
	}
	if !hasCross {
		t.Error("authorized DM: expected ❌ reaction (ACP invoked, binary not found)")
	}
}

// ============================================================
// T77 — Prompt sends sessionId matching the channel's session
// ============================================================

func TestDiscordACPSessionIDMatchesChannelID(t *testing.T) {
	const wantSessionID = "sess-ch-session-check"

	sess, srv := newACPSession(t, "ch-session-check", wantSessionID)
	defer srv.close()

	var gotSessionID string
	go func() {
		req := srv.readRequest(t)
		var params struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(req.Params, &params)
		gotSessionID = params.SessionID
		srv.sendNotification("session/update", map[string]any{
			"sessionId": wantSessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "ok"},
			},
		})
		srv.sendResponse(rawIDToInt64(req.ID), map[string]any{"status": "completed"})
	}()

	rt := &mockDiscordRT{}
	dgo := newMockDiscordSession(rt)
	sess.handleWithACP(dgo, "ch-session-check", "msg-sid", "ping", nil, nil, nil, nil, slog.Default())

	if gotSessionID != wantSessionID {
		t.Errorf("prompt session_id = %q, want %q", gotSessionID, wantSessionID)
	}
}
