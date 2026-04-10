package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBootstrapHandlerIsOneTimeOnly(t *testing.T) {
	bh := newBootstrapHandler([]byte("fake-p12-data"))

	req1 := httptest.NewRequest("GET", "/bootstrap", nil)
	w1 := httptest.NewRecorder()
	bh.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	req2 := httptest.NewRequest("GET", "/bootstrap", nil)
	w2 := httptest.NewRecorder()
	bh.ServeHTTP(w2, req2)
	if w2.Code == http.StatusOK {
		t.Fatalf("second request: expected non-200, got 200")
	}
}
