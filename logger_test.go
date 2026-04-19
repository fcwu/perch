package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoggerWritesJSON(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	var buf bytes.Buffer
	l := newLogger(&buf, nil)
	l.Info("test message", "key", "value")

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if entry["msg"] != "test message" {
		t.Errorf("expected msg='test message', got %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key='value', got %v", entry["key"])
	}
}

func TestLoggerTextFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	var buf bytes.Buffer
	l := newLogger(&buf, nil)
	l.Info("hello world", "user", "alice")
	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected 'alice' in output, got: %s", out)
	}
}

func TestLoggerJSONFormatFields(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	var buf bytes.Buffer
	l := newLogger(&buf, nil)
	l.Info("query_start", "user_id", "123", "session_id", "abc", "query", "test")

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	for _, key := range []string{"user_id", "session_id", "query"} {
		if entry[key] == nil {
			t.Errorf("expected key %q in JSON output", key)
		}
	}
}
