package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLoggerWritesJSON(t *testing.T) {
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
