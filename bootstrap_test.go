package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapHandlerIsOneTimeOnly(t *testing.T) {
	bh := newBootstrapHandler([]byte("fake-p12-data"), "")

	req1 := httptest.NewRequest("GET", "/bootstrap", nil)
	w1 := httptest.NewRecorder()
	bh.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	req2 := httptest.NewRequest("GET", "/bootstrap", nil)
	w2 := httptest.NewRecorder()
	bh.ServeHTTP(w2, req2)
	if w2.Code != http.StatusGone {
		t.Fatalf("second request: expected 410, got %d", w2.Code)
	}
}

func TestBootstrapHandlerPersistsUsedState(t *testing.T) {
	dir := t.TempDir()
	usedFile := filepath.Join(dir, "bootstrap.used")

	// First handler: use it
	bh1 := newBootstrapHandler([]byte("fake-p12-data"), usedFile)
	req := httptest.NewRequest("GET", "/bootstrap", nil)
	w := httptest.NewRecorder()
	bh1.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}
	if _, err := os.Stat(usedFile); err != nil {
		t.Fatalf("used file not created: %v", err)
	}

	// Second handler (simulates restart): should see file and block immediately
	bh2 := newBootstrapHandler([]byte("fake-p12-data"), usedFile)
	req2 := httptest.NewRequest("GET", "/bootstrap", nil)
	w2 := httptest.NewRecorder()
	bh2.ServeHTTP(w2, req2)
	if w2.Code != http.StatusGone {
		t.Fatalf("after restart: expected 410, got %d", w2.Code)
	}
}
