package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminLoginCorrectToken(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "secret123")
	a := newAdminAuth()

	body, _ := json.Marshal(map[string]string{"token": "secret123"})
	req := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "perch_admin" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected perch_admin cookie to be set")
	}
}

func TestAdminLoginWrongToken(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "secret123")
	a := newAdminAuth()

	body, _ := json.Marshal(map[string]string{"token": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "perch_admin" {
			t.Error("should not set perch_admin cookie on wrong token")
		}
	}
}


func TestAdminMiddlewareMissingCookie(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "tok")
	a := newAdminAuth()

	protected := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/history", nil)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing cookie, got %d", w.Code)
	}
}
