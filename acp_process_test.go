package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as a fake claude-agent-acp subprocess for EnsureRunning tests.
// When GO_TEST_ACP_HELPER=1, the test binary acts as the ACP server instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_ACP_HELPER") == "1" {
		runFakeACPHelper()
		return
	}
	os.Exit(m.Run())
}

// runFakeACPHelper speaks ACP JSON-RPC over stdin/stdout, responding to the
// three handshake methods and session/prompt with a fixed "pong" reply.
func runFakeACPHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg acpMsg
		if err := json.Unmarshal([]byte(scanner.Text()), &msg); err != nil || msg.ID == nil {
			continue
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}}
		case "session/new":
			result = map[string]any{"sessionId": "helper-session-001"}
		case "session/set_mode":
			result = map[string]any{}
		case "session/prompt":
			notif, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "helper-session-001",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "pong"},
					},
				},
			})
			fmt.Fprintf(os.Stdout, "%s\n", notif)
			result = map[string]any{"status": "completed"}
		default:
			result = map[string]any{}
		}
		resp, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
		fmt.Fprintf(os.Stdout, "%s\n", resp)
	}
}

// fakeACPServer simulates a claude-agent-acp subprocess via in-process io.Pipe pairs.
// Callers write protocol responses to serverW; the ACPProcess reads from serverR.
// The ACPProcess writes requests to clientR via clientW.
type fakeACPServer struct {
	// serverW → ACPProcess reads from here (simulates subprocess stdout)
	// clientR  ← ACPProcess writes here (simulates subprocess stdin)
	serverW io.WriteCloser
	clientR io.ReadCloser

	dec *json.Decoder // reads what ACPProcess sent (from clientR)
}

// newFakeACPServer creates an ACPProcess backed by in-process pipes and a server helper.
func newFakeACPServer(t *testing.T) (*ACPProcess, *fakeACPServer) {
	t.Helper()

	// Pipe for subprocess stdout: server writes, process reads.
	stdoutR, stdoutW := io.Pipe()
	// Pipe for subprocess stdin: process writes, server reads.
	stdinR, stdinW := io.Pipe()

	proc := &ACPProcess{
		executable: "fake-claude-agent-acp",
		workdir:    t.TempDir(),
		logger:     slog.Default(),
		pending:    make(map[int64]chan acpMsg),
	}
	// Inject the pipe ends directly (bypasses exec.Command).
	proc.stdinW = stdinW
	proc.running = false // not yet started (readLoop not running)

	srv := &fakeACPServer{
		serverW: stdoutW,
		clientR: stdinR,
		dec:     json.NewDecoder(stdinR),
	}

	// Start the readLoop with the server-side stdout pipe.
	go proc.readLoop(stdoutR)

	return proc, srv
}

// sendResponse writes a JSON-RPC response for the given request ID.
func (s *fakeACPServer) sendResponse(id int64, result any) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	_, _ = s.serverW.Write(append(data, '\n'))
}

// sendNotification writes a JSON-RPC notification (no ID).
func (s *fakeACPServer) sendNotification(method string, params any) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	_, _ = s.serverW.Write(append(data, '\n'))
}

// sendError writes a JSON-RPC error response.
func (s *fakeACPServer) sendError(id int64, msg string) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": -1, "message": msg},
	})
	_, _ = s.serverW.Write(append(data, '\n'))
}

// readRequest reads the next JSON-RPC request sent by the ACPProcess.
func (s *fakeACPServer) readRequest(t *testing.T) acpMsg {
	t.Helper()
	var msg acpMsg
	if err := s.dec.Decode(&msg); err != nil {
		t.Fatalf("fakeACPServer readRequest: %v", err)
	}
	return msg
}

// close signals end of subprocess stdout (simulates process exit).
func (s *fakeACPServer) close() {
	_ = s.serverW.Close()
	_ = s.clientR.Close()
}

// ============================================================
// Helpers to run the ACP handshake server side.
// ============================================================

// serveHandshake responds to initialize, session/new, and session/set_mode in a goroutine,
// returning the assigned session ID.
func serveHandshake(t *testing.T, srv *fakeACPServer, sessionID string) {
	t.Helper()
	go func() {
		// initialize
		req := srv.readRequest(t)
		if req.Method != "initialize" {
			t.Errorf("expected initialize, got %q", req.Method)
		}
		srv.sendResponse(*req.ID, map[string]any{
			"protocolVersion": 1,
			"agentCapabilities": map[string]any{},
		})

		// session/new
		req = srv.readRequest(t)
		if req.Method != "session/new" {
			t.Errorf("expected session/new, got %q", req.Method)
		}
		srv.sendResponse(*req.ID, map[string]any{"sessionId": sessionID})

		// session/set_mode (bypassPermissions)
		req = srv.readRequest(t)
		if req.Method != "session/set_mode" {
			t.Errorf("expected session/set_mode, got %q", req.Method)
		}
		srv.sendResponse(*req.ID, map[string]any{})
	}()
}

// ============================================================
// Tests
// ============================================================

// TestACPProcess_Handshake verifies that Start() sends initialize + session/new + session/set_mode
// and stores the returned sessionId.
func TestACPProcess_Handshake(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true // stdin/stdout already wired

	ctx := context.Background()
	serveHandshake(t, srv, "sess-handshake-001")

	// Drive the handshake manually via call().
	_, err := proc.call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "perch", "version": "1.0"},
		"clientCapabilities": map[string]any{},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	result, err := proc.call(ctx, "session/new", map[string]any{
		"cwd": "", "mcpServers": []any{},
	})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &sess); err != nil || sess.SessionID != "sess-handshake-001" {
		t.Errorf("unexpected sessionId: %s", result)
	}

	// session/set_mode
	if _, err := proc.call(ctx, "session/set_mode", map[string]any{
		"sessionId": sess.SessionID, "modeId": "bypassPermissions",
	}); err != nil {
		t.Fatalf("session/set_mode: %v", err)
	}
}

// TestACPProcess_Prompt_Success verifies that Prompt() accumulates agent_message_chunk
// notifications and returns the combined text when the response arrives.
func TestACPProcess_Prompt_Success(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true
	proc.sessionID = "sess-prompt-001"

	ctx := context.Background()

	go func() {
		// Read the prompt request.
		req := srv.readRequest(t)
		if req.Method != "session/prompt" {
			t.Errorf("expected session/prompt, got %q", req.Method)
		}

		// Stream two chunks via session/update notifications.
		srv.sendNotification("session/update", map[string]any{
			"sessionId": "sess-prompt-001",
			"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Hello, "}},
		})
		srv.sendNotification("session/update", map[string]any{
			"sessionId": "sess-prompt-001",
			"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "world!"}},
		})

		// Send the final response (completion).
		srv.sendResponse(*req.ID, map[string]any{"status": "completed"})
	}()

	text, err := proc.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if text != "Hello, world!" {
		t.Errorf("got %q, want %q", text, "Hello, world!")
	}
}

// TestACPProcess_Prompt_Error verifies that an RPC error from the subprocess
// is surfaced as an error from Prompt().
func TestACPProcess_Prompt_Error(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true
	proc.sessionID = "sess-err-001"

	ctx := context.Background()

	go func() {
		req := srv.readRequest(t)
		srv.sendError(*req.ID, "agent crashed")
	}()

	_, err := proc.Prompt(ctx, "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "agent crashed") {
		t.Errorf("error should mention 'agent crashed', got: %v", err)
	}
}

// TestACPProcess_Prompt_ContextCancel verifies that cancelling the context
// causes Prompt() to return an error and marks the process as not running.
func TestACPProcess_Prompt_ContextCancel(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true
	proc.sessionID = "sess-cancel-001"

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Read the request then do nothing (simulate hang).
		srv.readRequest(t)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := proc.Prompt(ctx, "hello")
	if err == nil {
		t.Fatal("expected error after cancel, got nil")
	}
}

// TestACPProcess_ProcessExit verifies that when the subprocess exits (pipe closes),
// pending call() returns an error and IsRunning() becomes false.
func TestACPProcess_ProcessExit(t *testing.T) {
	proc, srv := newFakeACPServer(t)

	proc.running = true
	proc.sessionID = "sess-exit-001"

	ctx := context.Background()

	// Close the server pipe (simulate process exit) before the request is served.
	go func() {
		time.Sleep(20 * time.Millisecond)
		srv.close() // closes serverW → readLoop sees EOF
	}()

	_, err := proc.call(ctx, "session/prompt", map[string]any{})
	if err == nil {
		t.Fatal("expected error after process exit, got nil")
	}
	// Give the readLoop goroutine time to update running.
	time.Sleep(50 * time.Millisecond)
	if proc.IsRunning() {
		t.Error("IsRunning should be false after process exit")
	}
}

// TestACPProcess_RequestFormat verifies that Prompt() sends the correct JSON-RPC
// fields: method="session/prompt", sessionId, prompt array.
func TestACPProcess_RequestFormat(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true
	proc.sessionID = "sess-fmt-001"

	ctx := context.Background()

	go func() {
		req := srv.readRequest(t)

		// Verify method.
		if req.Method != "session/prompt" {
			t.Errorf("method = %q, want %q", req.Method, "session/prompt")
		}

		// Verify params.
		var params struct {
			SessionID string `json:"sessionId"`
			Prompt    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"prompt"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("unmarshal params: %v", err)
		}
		if params.SessionID != "sess-fmt-001" {
			t.Errorf("sessionId = %q, want %q", params.SessionID, "sess-fmt-001")
		}
		if len(params.Prompt) == 0 || params.Prompt[0].Type != "text" || params.Prompt[0].Text != "test input" {
			t.Errorf("unexpected prompt: %+v", params.Prompt)
		}

		srv.sendResponse(*req.ID, map[string]any{"status": "completed"})
	}()

	if _, err := proc.Prompt(ctx, "test input"); err != nil {
		// Error is ok here (no chunks), we only care about the request format.
		_ = fmt.Sprintf("prompt err (expected): %v", err)
	}
}

// TestACPProcess_PromptWithContent_ImageBlock asserts that PromptWithContent serialises
// a [text, image] content slice into the session/prompt body as the spec requires.
func TestACPProcess_PromptWithContent_ImageBlock(t *testing.T) {
	proc, srv := newFakeACPServer(t)
	defer srv.close()

	proc.running = true
	proc.sessionID = "sess-img-001"

	ctx := context.Background()

	go func() {
		req := srv.readRequest(t)

		if req.Method != "session/prompt" {
			t.Errorf("method = %q, want %q", req.Method, "session/prompt")
		}

		var params struct {
			SessionID string       `json:"sessionId"`
			Prompt    []ACPContent `json:"prompt"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("unmarshal params: %v", err)
		}
		if params.SessionID != "sess-img-001" {
			t.Errorf("sessionId = %q, want %q", params.SessionID, "sess-img-001")
		}
		if len(params.Prompt) != 2 {
			t.Fatalf("len(prompt) = %d, want 2", len(params.Prompt))
		}
		if params.Prompt[0].Type != "text" || params.Prompt[0].Text != "what's wrong?" {
			t.Errorf("block[0] = %+v, want text/'what's wrong?'", params.Prompt[0])
		}
		if params.Prompt[1].Type != "image" || params.Prompt[1].Data != "AAAA" || params.Prompt[1].MimeType != "image/png" {
			t.Errorf("block[1] = %+v, want flat image-png/AAAA", params.Prompt[1])
		}

		srv.sendResponse(*req.ID, map[string]any{"status": "completed"})
	}()

	blocks := []ACPContent{
		{Type: "text", Text: "what's wrong?"},
		{Type: "image", Data: "AAAA", MimeType: "image/png"},
	}
	if _, err := proc.PromptWithContent(ctx, blocks, nil, nil, nil); err != nil {
		_ = fmt.Sprintf("prompt err (expected): %v", err)
	}
}

// TestACPProcess_EnsureRunning_NoDeadlock is a regression test for the deadlock where
// Start() held p.mu via defer while calling call(), which also needs p.mu.
// It runs a real subprocess (the test binary itself in helper mode) to exercise
// the full exec + handshake path that the in-process pipe tests bypass.
func TestACPProcess_EnsureRunning_NoDeadlock(t *testing.T) {
	t.Setenv("GO_TEST_ACP_HELPER", "1")

	proc := &ACPProcess{
		executable: os.Args[0],
		workdir:    t.TempDir(),
		logger:     slog.Default(),
		pending:    make(map[int64]chan acpMsg),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := proc.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if !proc.IsRunning() {
		t.Fatal("IsRunning should be true after EnsureRunning")
	}
	if proc.sessionID != "helper-session-001" {
		t.Errorf("sessionID = %q, want %q", proc.sessionID, "helper-session-001")
	}

	text, err := proc.Prompt(ctx, "ping")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if text != "pong" {
		t.Errorf("text = %q, want %q", text, "pong")
	}

	proc.Stop()
}
