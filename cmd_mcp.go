package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// runMCPServer is the entry point for `./perch mcp`. It reads identity from
// PERCH_USER_ID / PERCH_CONV_ID / PERCH_DB_PATH, opens a read-write SQLite
// handle, and serves a minimal stdio MCP server exposing three scheduler
// tools. Returns the desired process exit code.
func runMCPServer() int {
	userID := os.Getenv("PERCH_USER_ID")
	convID := os.Getenv("PERCH_CONV_ID")
	dbPath := os.Getenv("PERCH_DB_PATH")
	if userID == "" {
		fmt.Fprintln(os.Stderr, "perch mcp: PERCH_USER_ID is required")
		return 2
	}
	if convID == "" {
		fmt.Fprintln(os.Stderr, "perch mcp: PERCH_CONV_ID is required")
		return 2
	}
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "perch mcp: PERCH_DB_PATH is required")
		return 2
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perch mcp: open store %q: %v\n", dbPath, err)
		return 1
	}
	defer store.Close()

	srv := &mcpServer{
		store:  store,
		userID: userID,
		convID: convID,
	}
	srv.run(os.Stdin, os.Stdout)
	return 0
}

// mcpServer implements the subset of the MCP protocol perch needs: initialize,
// tools/list, tools/call. Communication is JSON-RPC 2.0, line-delimited JSON
// over stdin/stdout. All identity is bound from env at startup; tool args
// CANNOT override user_id or conversation_id.
type mcpServer struct {
	store  *Store
	userID string
	convID string

	mu sync.Mutex
}

type mcpRPC struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *mcpServer) run(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	w := bufio.NewWriter(out)
	defer w.Flush()
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg mcpRPC
		if err := json.Unmarshal(line, &msg); err != nil {
			s.writeError(w, nil, -32700, "parse error: "+err.Error())
			continue
		}
		if msg.Method == "" {
			// We don't make any outgoing requests; an unsolicited
			// response is unexpected — drop.
			continue
		}
		s.handle(w, msg)
		w.Flush()
	}
}

func (s *mcpServer) writeRaw(w *bufio.Writer, msg mcpRPC) {
	msg.JSONRPC = "2.0"
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	w.Write(b)
	w.WriteByte('\n')
}

func (s *mcpServer) writeError(w *bufio.Writer, id json.RawMessage, code int, message string) {
	s.writeRaw(w, mcpRPC{
		ID:    id,
		Error: &mcpError{Code: code, Message: message},
	})
}

func (s *mcpServer) writeResult(w *bufio.Writer, id json.RawMessage, result any) {
	b, err := json.Marshal(result)
	if err != nil {
		s.writeError(w, id, -32603, "marshal result: "+err.Error())
		return
	}
	s.writeRaw(w, mcpRPC{ID: id, Result: b})
}

func (s *mcpServer) handle(w *bufio.Writer, req mcpRPC) {
	switch req.Method {
	case "initialize":
		s.writeResult(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "perch-mcp",
				"version": "1.0",
			},
		})
	case "notifications/initialized":
		// notification, no response
	case "tools/list":
		s.writeResult(w, req.ID, map[string]any{"tools": s.toolsList()})
	case "tools/call":
		s.handleToolsCall(w, req)
	default:
		if len(req.ID) > 0 {
			s.writeError(w, req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func (s *mcpServer) toolsList() []map[string]any {
	return []map[string]any{
		{
			"name":        "schedule_message",
			"description": "Schedule a chat message to be delivered to the current conversation later. Either set hour+minute (with optional repeat) for a daily schedule, or one_shot_at as epoch milliseconds for a one-shot.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":      map[string]any{"type": "string", "description": "Prompt text to send"},
					"hour":        map[string]any{"type": "integer", "minimum": 0, "maximum": 23, "description": "Hour-of-day for daily schedule"},
					"minute":      map[string]any{"type": "integer", "minimum": 0, "maximum": 59, "description": "Minute-of-hour for daily schedule"},
					"repeat":      map[string]any{"type": "boolean", "description": "Whether daily schedule repeats; ignored for one-shot"},
					"one_shot_at": map[string]any{"type": "integer", "description": "One-shot fire time as epoch milliseconds (must be in the future)"},
				},
				"required": []string{"prompt"},
			},
		},
		{
			"name":        "list_schedules",
			"description": "List enabled schedules for the current conversation.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "cancel_schedule",
			"description": "Cancel a previously created schedule by id.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Schedule id"},
				},
				"required": []string{"id"},
			},
		},
	}
}

func (s *mcpServer) handleToolsCall(w *bufio.Writer, req mcpRPC) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.writeError(w, req.ID, -32602, "bad params: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch p.Name {
	case "schedule_message":
		s.toolScheduleMessage(ctx, w, req.ID, p.Arguments)
	case "list_schedules":
		s.toolListSchedules(ctx, w, req.ID)
	case "cancel_schedule":
		s.toolCancelSchedule(ctx, w, req.ID, p.Arguments)
	default:
		s.writeError(w, req.ID, -32601, "unknown tool: "+p.Name)
	}
}

// toolResult wraps a JSON-encodable payload as MCP tool content.
func toolResult(payload any) map[string]any {
	b, _ := json.Marshal(payload)
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(b)},
		},
	}
}

func toolError(message string) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]any{
			{"type": "text", "text": message},
		},
	}
}

func (s *mcpServer) toolScheduleMessage(ctx context.Context, w *bufio.Writer, id json.RawMessage, args json.RawMessage) {
	var a struct {
		Prompt    string `json:"prompt"`
		Hour      *int   `json:"hour,omitempty"`
		Minute    *int   `json:"minute,omitempty"`
		Repeat    bool   `json:"repeat,omitempty"`
		OneShotAt int64  `json:"one_shot_at,omitempty"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeResult(w, id, toolError("invalid arguments: "+err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	if err := validateChatScheduleInput(a.Prompt, a.Hour, a.Minute, a.OneShotAt, now); err != nil {
		s.writeResult(w, id, toolError(err.Error()))
		return
	}
	sch := &ChatSchedule{
		UserID:         s.userID,
		ConversationID: s.convID,
		Hour:           a.Hour,
		Minute:         a.Minute,
		Repeat:         a.Repeat,
		OneShotAt:      a.OneShotAt,
		Prompt:         a.Prompt,
		Enabled:        true,
	}
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		done <- result{err: s.store.InsertChatSchedule(sch)}
	}()
	select {
	case <-ctx.Done():
		s.writeResult(w, id, toolError("timeout"))
	case r := <-done:
		if r.err != nil {
			s.writeResult(w, id, toolError("insert failed: "+r.err.Error()))
			return
		}
		s.writeResult(w, id, toolResult(map[string]any{"id": sch.ID}))
	}
}

func (s *mcpServer) toolListSchedules(ctx context.Context, w *bufio.Writer, id json.RawMessage) {
	type result struct {
		rows []ChatSchedule
		err  error
	}
	done := make(chan result, 1)
	go func() {
		all, err := s.store.ListChatSchedules(s.userID, s.convID)
		// Filter to enabled-only per spec.
		filtered := make([]ChatSchedule, 0, len(all))
		for _, r := range all {
			if r.Enabled {
				filtered = append(filtered, r)
			}
		}
		done <- result{rows: filtered, err: err}
	}()
	select {
	case <-ctx.Done():
		s.writeResult(w, id, toolError("timeout"))
	case r := <-done:
		if r.err != nil {
			s.writeResult(w, id, toolError("list failed: "+r.err.Error()))
			return
		}
		if r.rows == nil {
			r.rows = []ChatSchedule{}
		}
		s.writeResult(w, id, toolResult(map[string]any{"schedules": r.rows}))
	}
}

func (s *mcpServer) toolCancelSchedule(ctx context.Context, w *bufio.Writer, id json.RawMessage, args json.RawMessage) {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeResult(w, id, toolError("invalid arguments: "+err.Error()))
		return
	}
	if a.ID == "" {
		s.writeResult(w, id, toolError("id is required"))
		return
	}
	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, err := s.store.DeleteChatSchedule(a.ID, s.userID, s.convID)
		done <- result{ok: ok, err: err}
	}()
	select {
	case <-ctx.Done():
		s.writeResult(w, id, toolError("timeout"))
	case r := <-done:
		if r.err != nil {
			s.writeResult(w, id, toolError("delete failed: "+r.err.Error()))
			return
		}
		s.writeResult(w, id, toolResult(map[string]any{"deleted": r.ok}))
	}
}
