package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// ACPProcess manages a long-lived claude-agent-acp subprocess per Discord channel.
// Communication is ACP JSON-RPC 2.0 over stdin/stdout (line-delimited JSON).
// Multi-turn conversation context is preserved across Prompt() calls within one session.
// All exported methods are safe for concurrent use.
type ACPProcess struct {
	executable string
	workdir    string
	logger     *slog.Logger

	// lifecycle
	mu        sync.Mutex
	initMu    sync.Mutex // serializes EnsureRunning→Start to prevent concurrent init races
	cmd       *exec.Cmd
	stdinW    io.WriteCloser
	running   bool
	sessionID string

	// request/response correlation
	nextID  atomic.Int64
	pendMu  sync.Mutex
	pending map[int64]chan acpMsg

	// streaming chunks from agent_message_chunk notifications (one prompt at a time)
	chunkMu  sync.Mutex
	chunkBuf strings.Builder
}

// acpMsg is the wire format for ACP JSON-RPC 2.0 messages.
type acpMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpRPCError    `json:"error,omitempty"`
}

type acpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewACPProcess creates an ACPProcess that will fork executable (default from
// ACP_EXECUTABLE env var, then "claude-agent-acp").
func NewACPProcess(executable, workdir string, logger *slog.Logger) *ACPProcess {
	if executable == "" {
		executable = os.Getenv("ACP_EXECUTABLE")
	}
	if executable == "" {
		executable = "claude-agent-acp"
	}
	return &ACPProcess{
		executable: executable,
		workdir:    workdir,
		logger:     logger,
		pending:    make(map[int64]chan acpMsg),
	}
}

// IsRunning reports whether the subprocess is alive.
func (p *ACPProcess) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// EnsureRunning starts the subprocess if it is not already running.
// initMu serializes concurrent callers so only one goroutine runs Start()
// at a time; others wait and then find the process already running.
func (p *ACPProcess) EnsureRunning(ctx context.Context) error {
	p.initMu.Lock()
	defer p.initMu.Unlock()
	if p.IsRunning() {
		return nil
	}
	return p.Start(ctx)
}

// Start forks the subprocess and performs the ACP handshake (initialize + new_session).
// Calling Start when already running is a no-op.
func (p *ACPProcess) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}

	cmd := exec.Command(p.executable) // lifecycle managed explicitly; not ctx-bound
	if p.workdir != "" {
		cmd.Dir = p.workdir
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("acp: stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("acp: start %q: %w", p.executable, err)
	}

	p.cmd = cmd
	p.stdinW = stdinPipe
	p.running = true

	go p.readLoop(stdoutPipe)
	p.mu.Unlock() // release before handshake — call() also acquires p.mu

	// ACP handshake: initialize (protocolVersion must be an integer)
	if _, err := p.call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "perch", "version": "1.0"},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		p.Stop()
		return fmt.Errorf("acp: initialize: %w", err)
	}

	// ACP handshake: session/new (permissionMode comes from settings, set via session/set_mode below)
	result, err := p.call(ctx, "session/new", map[string]any{
		"cwd":        p.workdir,
		"mcpServers": []any{},
	})
	if err != nil {
		p.Stop()
		return fmt.Errorf("acp: session/new: %w", err)
	}

	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &sess); err != nil || sess.SessionID == "" {
		p.Stop()
		return fmt.Errorf("acp: session/new: no sessionId in response: %s", result)
	}
	p.mu.Lock()
	p.sessionID = sess.SessionID
	p.mu.Unlock()

	// Switch to bypassPermissions so the bot can use tools without interactive approval.
	if _, err := p.call(ctx, "session/set_mode", map[string]any{
		"sessionId": p.sessionID,
		"modeId":    "bypassPermissions",
	}); err != nil {
		p.logger.Warn("acp: session/set_mode bypassPermissions failed (continuing)", "err", err)
	}

	p.logger.Info("ACP process started", "sessionId", p.sessionID, "executable", p.executable)
	return nil
}

// Prompt sends text to the agent and returns the accumulated response text.
// On ctx cancellation or error the subprocess is killed; EnsureRunning will restart it.
func (p *ACPProcess) Prompt(ctx context.Context, text string) (string, error) {
	// Reset chunk accumulator before sending the prompt.
	p.chunkMu.Lock()
	p.chunkBuf.Reset()
	p.chunkMu.Unlock()

	p.mu.Lock()
	sessionID := p.sessionID
	p.mu.Unlock()

	if _, err := p.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}); err != nil {
		if ctx.Err() != nil {
			p.mu.Lock()
			p.killLocked()
			p.mu.Unlock()
		}
		return "", err
	}

	p.chunkMu.Lock()
	out := p.chunkBuf.String()
	p.chunkMu.Unlock()
	return out, nil
}

// Stop kills the subprocess immediately.
func (p *ACPProcess) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.killLocked()
}

// killLocked kills the subprocess. Must be called with p.mu held.
func (p *ACPProcess) killLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.stdinW != nil {
		_ = p.stdinW.Close()
		p.stdinW = nil
	}
	p.running = false
	p.sessionID = ""
}

// call sends a JSON-RPC request and waits for the matching response.
// Intermediate notifications (e.g. session/update) are handled by readLoop.
func (p *ACPProcess) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := p.nextID.Add(1)

	req := acpMsg{
		JSONRPC: "2.0",
		Method:  method,
	}
	req.ID = &id
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("acp: marshal params: %w", err)
		}
		req.Params = b
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	// Register a response channel before writing so we don't miss the reply.
	ch := make(chan acpMsg, 1)
	p.pendMu.Lock()
	p.pending[id] = ch
	p.pendMu.Unlock()

	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		p.pendMu.Lock()
		delete(p.pending, id)
		p.pendMu.Unlock()
		return nil, fmt.Errorf("acp: process not running")
	}
	_, writeErr := p.stdinW.Write(data)
	p.mu.Unlock()

	if writeErr != nil {
		p.pendMu.Lock()
		delete(p.pending, id)
		p.pendMu.Unlock()
		return nil, fmt.Errorf("acp: write request: %w", writeErr)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("acp: %s error: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		p.pendMu.Lock()
		delete(p.pending, id)
		p.pendMu.Unlock()
		return nil, ctx.Err()
	}
}

// readLoop continuously reads stdout from the subprocess, routing responses to
// pending callers and accumulating agent_message_chunk notifications.
// It exits when the subprocess closes stdout (normal shutdown or kill).
func (p *ACPProcess) readLoop(r io.Reader) {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MB line buffer for large responses

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg acpMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			p.logger.Debug("ACP: unparseable line", "line", line[:min(len(line), 200)])
			continue
		}

		switch {
		case msg.ID != nil && msg.Method != "":
			// Incoming request from agent (e.g. session/request_permission).
			// Auto-approve with bypassPermissions so the bot runs unattended.
			resp := acpMsg{
				JSONRPC: "2.0",
				ID:      msg.ID,
			}
			if msg.Method == "session/request_permission" {
				b, _ := json.Marshal(map[string]any{
					"outcome": map[string]any{"outcome": "selected", "optionId": "bypassPermissions"},
				})
				resp.Result = b
			} else {
				// Unknown agent→client request: return empty success.
				resp.Result = json.RawMessage(`{}`)
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			p.mu.Lock()
			if p.stdinW != nil {
				p.stdinW.Write(data) //nolint:errcheck
			}
			p.mu.Unlock()

		case msg.ID != nil:
			// Response to a pending call.
			p.pendMu.Lock()
			ch := p.pending[*msg.ID]
			delete(p.pending, *msg.ID)
			p.pendMu.Unlock()
			if ch != nil {
				ch <- msg
			}

		case msg.Method == "session/update":
			// Streaming update notification. Extract agent_message_chunk text.
			var params struct {
				Update struct {
					SessionUpdate string `json:"sessionUpdate"`
					Content       struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err == nil &&
				params.Update.SessionUpdate == "agent_message_chunk" &&
				params.Update.Content.Type == "text" {
				p.chunkMu.Lock()
				p.chunkBuf.WriteString(params.Update.Content.Text)
				p.chunkMu.Unlock()
			}
		}
	}

	// Reap the subprocess to prevent zombies.
	if cmd != nil {
		_ = cmd.Wait()
	}

	// Subprocess has closed stdout — mark as not running and drain pending callers.
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	p.pendMu.Lock()
	for id, ch := range p.pending {
		ch <- acpMsg{Error: &acpRPCError{Message: "process exited"}}
		delete(p.pending, id)
	}
	p.pendMu.Unlock()

	p.logger.Info("ACP process exited", "executable", p.executable)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
