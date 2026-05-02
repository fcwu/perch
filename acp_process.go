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
	"time"
)

// ACPContent is a single content block in an ACP session/prompt request.
// The on-the-wire shape is flat per ACP schema's ImageContent / TextContent
// (NOT Anthropic API's nested {source:{...}} shape):
//   text:  {"type":"text","text":"..."}
//   image: {"type":"image","data":"<base64>","mimeType":"image/png"}
type ACPContent struct {
	Type     string `json:"type"`               // "text" | "image"
	Text     string `json:"text,omitempty"`     // when Type=="text"
	Data     string `json:"data,omitempty"`     // raw base64 (no data: prefix), when Type=="image"
	MimeType string `json:"mimeType,omitempty"` // e.g. "image/png", when Type=="image"
}

// ACPProcess manages a long-lived ACP subprocess (claude-agent-acp, opencode acp,
// or any other ACP-compatible binary) per session key.
// Communication is ACP JSON-RPC 2.0 over stdin/stdout (line-delimited JSON).
// Multi-turn conversation context is preserved across Prompt() calls within one session.
// All exported methods are safe for concurrent use.
type ACPProcess struct {
	executable string
	args       []string // extra args before any user-supplied prompt args (e.g. ["acp","--log-level","WARN"] for opencode)
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
	chunkMu     sync.Mutex
	chunkBuf    strings.Builder
	chunkCb     func(string)      // optional per-prompt callback; called from readLoop goroutine
	toolStartCb func(string)      // called with tool name on session/update tool_call (pending)
	toolEndCb   func()            // called on session/update tool_call_update with status=completed
}

// acpMsg is the wire format for ACP JSON-RPC 2.0 messages.
type acpMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	// ID is RawMessage so we accept either integer (claude/opencode) or
	// string (codex sends UUIDs for agent→client requests like
	// session/request_permission). Pending-call tracking on perch's side
	// uses int64 (calls perch initiates), so int IDs are still parsed
	// numerically downstream; string IDs only flow through agent→client
	// dispatch where we echo the raw ID back.
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *acpRPCError    `json:"error,omitempty"`
}

type acpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// pickAutoApproveOption picks an optionId from a session/request_permission
// params payload. Different agents expose different options:
//   - claude-agent-acp historically accepts "bypassPermissions"
//   - codex-acp offers options like {optionId:"approved", kind:"allow_once"}
//
// Strategy: parse `options[]`, prefer the first with kind="allow_once", then
// "allow_always", then any non-reject option. Fall back to "bypassPermissions"
// if the params shape is unfamiliar (legacy claude path).
func pickAutoApproveOption(params json.RawMessage) string {
	var p struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if len(params) == 0 || json.Unmarshal(params, &p) != nil || len(p.Options) == 0 {
		return "bypassPermissions"
	}
	var fallback string
	for _, opt := range p.Options {
		switch opt.Kind {
		case "allow_once":
			return opt.OptionID
		case "allow_always":
			if fallback == "" {
				fallback = opt.OptionID
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	// No allow_* option found; pick the first non-reject as last resort.
	for _, opt := range p.Options {
		if opt.Kind != "reject_once" && opt.Kind != "reject_always" {
			return opt.OptionID
		}
	}
	return "bypassPermissions"
}

// NewACPProcess creates an ACPProcess for the given executable + args.
// The ACP_EXECUTABLE env var (and optional ACP_EXECUTABLE_ARGS JSON array)
// is honoured as a developer override — useful for pointing perch at a fork
// or a mock subprocess in tests. Caller (typically chat-API or IM adapter)
// should pass runtime.ACPExecutable + runtime.ACPArgs to drive selection
// from AGENT_RUNTIME.
func NewACPProcess(executable string, args []string, workdir string, logger *slog.Logger) *ACPProcess {
	if v := os.Getenv("ACP_EXECUTABLE"); v != "" {
		executable = v
	}
	if executable == "" {
		// Last-resort default: behave as before to keep dev workflows working
		// when neither runtime nor env is set (e.g. unit tests).
		executable = "claude-agent-acp"
	}
	if v := os.Getenv("ACP_EXECUTABLE_ARGS"); v != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			args = parsed
		} else if logger != nil {
			logger.Warn("ACP_EXECUTABLE_ARGS is not a JSON array; ignoring", "value", v, "err", err)
		}
	}
	return &ACPProcess{
		executable: executable,
		args:       args,
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

	cmd := exec.Command(p.executable, p.args...) // lifecycle managed explicitly; not ctx-bound
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
	t0 := time.Now()
	if _, err := p.call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "perch", "version": "1.0"},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		p.Stop()
		return fmt.Errorf("acp: initialize: %w", err)
	}
	p.logger.Debug("ACP handshake: initialize done", "elapsed", time.Since(t0).Round(time.Millisecond))

	// ACP handshake: session/new (permissionMode comes from settings, set via session/set_mode below)
	t1 := time.Now()
	result, err := p.call(ctx, "session/new", map[string]any{
		"cwd":        p.workdir,
		"mcpServers": []any{},
	})
	if err != nil {
		p.Stop()
		return fmt.Errorf("acp: session/new: %w", err)
	}
	p.logger.Debug("ACP handshake: session/new done", "elapsed", time.Since(t1).Round(time.Millisecond))

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

	p.logger.Info("ACP process started", "sessionId", p.sessionID, "executable", p.executable, "totalHandshake", time.Since(t0).Round(time.Millisecond))
	return nil
}

// Prompt sends text to the agent and returns the accumulated response text.
// On ctx cancellation or error the subprocess is killed; EnsureRunning will restart it.
func (p *ACPProcess) Prompt(ctx context.Context, text string) (string, error) {
	return p.PromptWithChunks(ctx, text, nil, nil, nil)
}

// PromptWithChunks is like Prompt but calls onChunk for each agent_message_chunk as it
// arrives (before the full response is available). onChunk may be nil.
// onToolStart is called with the tool name when a session/update tool_call event arrives.
// onToolEnd is called when a session/update tool_call_update with status=completed arrives.
func (p *ACPProcess) PromptWithChunks(ctx context.Context, text string, onChunk func(string), onToolStart func(string), onToolEnd func()) (string, error) {
	return p.PromptWithContent(ctx, []ACPContent{{Type: "text", Text: text}}, onChunk, onToolStart, onToolEnd)
}

// PromptWithContent sends a multi-block prompt (text + optional image blocks) to the agent
// and returns the accumulated response text. Use this for vision queries; for text-only see
// Prompt or PromptWithChunks.
func (p *ACPProcess) PromptWithContent(ctx context.Context, blocks []ACPContent, onChunk func(string), onToolStart func(string), onToolEnd func()) (string, error) {
	// Reset chunk accumulator and register callbacks before sending the prompt.
	p.chunkMu.Lock()
	p.chunkBuf.Reset()
	p.chunkCb = onChunk
	p.toolStartCb = onToolStart
	p.toolEndCb = onToolEnd
	p.chunkMu.Unlock()
	defer func() {
		p.chunkMu.Lock()
		p.chunkCb = nil
		p.toolStartCb = nil
		p.toolEndCb = nil
		p.chunkMu.Unlock()
	}()

	p.mu.Lock()
	sessionID := p.sessionID
	p.mu.Unlock()

	tPrompt := time.Now()
	if _, err := p.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    blocks,
	}); err != nil {
		if ctx.Err() != nil {
			p.mu.Lock()
			p.killLocked()
			p.mu.Unlock()
		}
		return "", err
	}
	p.logger.Debug("ACP prompt done", "elapsed", time.Since(tPrompt).Round(time.Millisecond), "blocks", len(blocks))

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
	idJSON, _ := json.Marshal(id)
	req.ID = idJSON
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
			p.logger.Debug("ACP: unparseable line", "err", err, "line", line[:min(len(line), 200)])
			continue
		}

		switch {
		case len(msg.ID) > 0 && msg.Method != "":
			// Incoming request from agent (e.g. session/request_permission).
			// Auto-approve so the bot runs unattended. The optionId differs
			// per runtime: claude offers "bypassPermissions"; codex offers
			// "approved" / "approved-execpolicy-amendment" / "abort". Pick
			// the first option whose kind is "allow_once" (or "allow_always"
			// as fallback) from the request's options array, defaulting to
			// "bypassPermissions" for legacy claude shape.
			resp := acpMsg{
				JSONRPC: "2.0",
				ID:      msg.ID,
			}
			if msg.Method == "session/request_permission" {
				optionId := pickAutoApproveOption(msg.Params)
				b, _ := json.Marshal(map[string]any{
					"outcome": map[string]any{"outcome": "selected", "optionId": optionId},
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

		case len(msg.ID) > 0:
			// Response to a pending call. Only int IDs are ours (perch
			// originates int IDs via nextID); string IDs from the agent's
			// own request namespace shouldn't reach here, but guard anyway.
			var id int64
			if err := json.Unmarshal(msg.ID, &id); err != nil {
				p.logger.Debug("ACP: response with non-int ID, ignoring", "id", string(msg.ID))
				continue
			}
			p.pendMu.Lock()
			ch := p.pending[id]
			delete(p.pending, id)
			p.pendMu.Unlock()
			if ch != nil {
				ch <- msg
			}

		case msg.Method == "session/update":
			// Streaming update notification.
			// claude-agent-acp 2.x sends:
			//   sessionUpdate: "agent_message_chunk" — content is an OBJECT {type,text}
			//   sessionUpdate: "tool_call"           — content is an ARRAY (status: pending)
			//   sessionUpdate: "tool_call_update"    — content is an ARRAY (status: completed when done)
			//   tool name lives at update._meta.claudeCode.toolName
			// Content shape varies, so decode it as RawMessage and parse per-case.
			var params struct {
				Update struct {
					SessionUpdate string          `json:"sessionUpdate"`
					Status        string          `json:"status"`
					Content       json.RawMessage `json:"content"`
					Meta          struct {
						ClaudeCode struct {
							ToolName string `json:"toolName"`
						} `json:"claudeCode"`
					} `json:"_meta"`
				} `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				break
			}
			switch params.Update.SessionUpdate {
			case "agent_message_chunk":
				var chunkContent struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(params.Update.Content, &chunkContent); err != nil {
					break
				}
				if chunkContent.Type == "text" {
					chunk := chunkContent.Text
					p.chunkMu.Lock()
					p.chunkBuf.WriteString(chunk)
					cb := p.chunkCb
					p.chunkMu.Unlock()
					if cb != nil {
						cb(chunk)
					}
				}
			case "tool_call":
				p.chunkMu.Lock()
				cb := p.toolStartCb
				p.chunkMu.Unlock()
				if cb != nil {
					cb(params.Update.Meta.ClaudeCode.ToolName)
				}
			case "tool_call_update":
				if params.Update.Status != "completed" {
					break
				}
				p.chunkMu.Lock()
				cb := p.toolEndCb
				p.chunkMu.Unlock()
				if cb != nil {
					cb()
				}
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
