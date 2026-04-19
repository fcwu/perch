package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHookHandlerInvalidMethod(t *testing.T) {
	h := newHookHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/hook", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHookHandlerInvalidJSON(t *testing.T) {
	h := newHookHandler(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHookHandlerValidEvent(t *testing.T) {
	var received HookEvent
	im := newIMManager(nil)
	im.addAdapter(&captureAdapter{onNotify: func(e HookEvent, _ string) { received = e }})

	h := newHookHandler(im, nil)
	body := `{"hook_event_name":"PreToolUse","tool_name":"Bash"}`
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if received.EventName != "PreToolUse" {
		t.Fatalf("expected PreToolUse event, got %q", received.EventName)
	}
	if received.ToolName != "Bash" {
		t.Fatalf("expected tool Bash, got %q", received.ToolName)
	}
}

func TestHookHandlerStopExtractsText(t *testing.T) {
	var lastText string
	im := newIMManager(nil)
	im.addAdapter(&captureAdapter{onNotify: func(_ HookEvent, t string) { lastText = t }})

	h := newHookHandler(im, nil)
	body := `{
		"hook_event_name": "Stop",
		"transcript": [
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "world"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	httptest.NewRecorder()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if lastText != "world" {
		t.Fatalf("expected lastText 'world', got %q", lastText)
	}
}

// captureAdapter is a test double for IMAdapter.
type captureAdapter struct {
	onNotify func(HookEvent, string)
}

func (c *captureAdapter) Start(_ *PTYManager) error { return nil }
func (c *captureAdapter) Stop()                     {}
func (c *captureAdapter) Notify(e HookEvent, text string) error {
	if c.onNotify != nil {
		c.onNotify(e, text)
	}
	return nil
}

func TestIMManagerSessionsReturnsNilWhenNoDiscord(t *testing.T) {
	im := newIMManager(nil)
	im.addAdapter(&captureAdapter{})
	if im.Sessions() != nil {
		t.Fatal("expected nil SessionProvider when no Discord adapter")
	}
}
