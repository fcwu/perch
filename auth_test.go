package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthNoneAllowsAll(t *testing.T) {
	mw := newAuthMiddleware("none", "")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthPasswordBlocksWithoutCookie(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200, got 200")
	}
}

func TestAuthPasswordAllowsLoginEndpoint(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("POST", "/login", nil)
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /login, got %d", w.Code)
	}
}

func TestAuthPasswordValidSession(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	token := mw.newSession()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/ws", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid session, got %d", w.Code)
	}
}
