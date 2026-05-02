package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helper: create a server with a real store and an in-process session manager
// stub. Returns (server, store) — caller is responsible for adding rows.
func newConvTestServer(t *testing.T) (*Server, *Store) {
	t.Helper()
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	pm := newPTYManager()
	srv := newServer(pm, nil, nil, nil, nil, nil, nil, nil, store, nil, nil)
	rt, _ := agentRuntimeByName("claude")
	srv.defaultRuntime = rt
	return srv, store
}

func doJSON(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func TestListConversationsCursor(t *testing.T) {
	srv, store := newConvTestServer(t)
	for _, id := range []string{"c1", "c2", "c3"} {
		if err := store.InsertConversation(id, "default", id, "claude", "claude-sonnet-4-6"); err != nil {
			t.Fatal(err)
		}
	}
	rr := doJSON(t, srv, http.MethodGet, "/api/conversations?limit=10", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Pinned     []ConversationRow `json:"pinned"`
		Recent     []ConversationRow `json:"recent"`
		NextBefore int64             `json:"next_before"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if len(resp.Recent) != 3 {
		t.Errorf("expected 3 recent rows, got %d", len(resp.Recent))
	}
}

func TestPatchConversationPin(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "default", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	rr := doJSON(t, srv, http.MethodPatch, "/api/conversations/c1", map[string]any{"pinned": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	conv, _ := store.GetConversation("c1", "default")
	if !conv.Pinned {
		t.Error("expected pinned=true after PATCH")
	}
}

func TestPatchConversationRuntimeRejectsUnknown(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "default", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	rr := doJSON(t, srv, http.MethodPatch, "/api/conversations/c1", map[string]any{"runtime": "no-such-runtime"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown runtime, got %d", rr.Code)
	}
}

func TestPatchConversationRuntimeAccepted(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "default", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	rr := doJSON(t, srv, http.MethodPatch, "/api/conversations/c1", map[string]any{
		"runtime": "opencode", "model": "opencode-default",
	})
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	conv, _ := store.GetConversation("c1", "default")
	if conv.Runtime != "opencode" || conv.Model != "opencode-default" {
		t.Errorf("expected opencode/opencode-default, got %s/%s", conv.Runtime, conv.Model)
	}
}

func TestPatchConversationCrossUserNotFound(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "alice", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	// resolveUserID returns "default" without an auth context, so a row owned
	// by "alice" must be hidden — PATCH should 404.
	rr := doJSON(t, srv, http.MethodPatch, "/api/conversations/c1", map[string]any{"pinned": true})
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-user PATCH, got %d", rr.Code)
	}
}

func TestListRuntimes(t *testing.T) {
	srv, _ := newConvTestServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/api/runtimes", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Runtimes []struct {
			ID           string   `json:"id"`
			Models       []string `json:"models"`
			DefaultModel string   `json:"default_model"`
			SupportsMCP  bool     `json:"supports_mcp"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if len(resp.Runtimes) == 0 {
		t.Fatal("no runtimes returned")
	}
	foundClaude := false
	for _, r := range resp.Runtimes {
		if r.ID == "claude" {
			foundClaude = true
			if !r.SupportsMCP {
				t.Error("expected claude.supports_mcp=true")
			}
			if len(r.Models) == 0 {
				t.Error("expected claude.models non-empty")
			}
		}
	}
	if !foundClaude {
		t.Error("claude runtime missing from /api/runtimes response")
	}
}

func TestChatScheduleCRUD(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "default", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	// Create one-shot
	rr := doJSON(t, srv, http.MethodPost, "/api/conversations/c1/schedules", map[string]any{
		"prompt":      "ping",
		"one_shot_at": int64(1<<62),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	var sch ChatSchedule
	if err := json.Unmarshal(rr.Body.Bytes(), &sch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sch.ID == "" {
		t.Fatal("expected schedule id in response")
	}

	// List
	rr = doJSON(t, srv, http.MethodGet, "/api/conversations/c1/schedules", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), sch.ID) {
		t.Error("schedule id missing from list response")
	}

	// Delete
	rr = doJSON(t, srv, http.MethodDelete, "/api/conversations/c1/schedules/"+sch.ID, nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Cross-user / cross-conv
	rr = doJSON(t, srv, http.MethodGet, "/api/conversations/no-such-conv/schedules", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown conv, got %d", rr.Code)
	}
}

func TestChatScheduleValidationErrors(t *testing.T) {
	srv, store := newConvTestServer(t)
	if err := store.InsertConversation("c1", "default", "T", "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	// Both daily and one-shot — invalid.
	rr := doJSON(t, srv, http.MethodPost, "/api/conversations/c1/schedules", map[string]any{
		"prompt": "ping", "hour": 9, "minute": 0, "one_shot_at": int64(1<<62),
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid combo, got %d", rr.Code)
	}
}
