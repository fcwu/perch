package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAuthPasswordBlocksAllEndpoints(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := []string{"/", "/ws", "/input", "/schedule", "/schedule/some-id"}
	for _, path := range protected {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mw.wrap(next).ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("path %s: expected 401 without session, got %d", path, w.Code)
		}
	}
}

func TestAuthPasswordBypassEndpoints(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// /login must be reachable without a session so users can authenticate
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("/login: expected non-401 (auth bypass), got 401")
	}
}


func TestAuthPasswordInjectsUserIDInContext(t *testing.T) {
	mw := newAuthMiddleware("password", "secret")
	token := mw.newSession()

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(ctxUserID).(string)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/chat", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	mw.wrap(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotUserID == "" {
		t.Error("expected ctxUserID to be injected in password auth mode")
	}
}

func TestAuthPasswordSessionCookieNotSecure(t *testing.T) {
	// password mode uses plain HTTP; the Secure flag would prevent the browser
	// from sending the cookie back over HTTP, breaking the login flow entirely.
	mw := newAuthMiddleware("password", "secret")
	req := httptest.NewRequest("POST", "/login", strings.NewReader(`{"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mw.handleLogin(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login expected 204, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" && c.Secure {
			t.Error("session cookie must not have Secure flag in password (plain HTTP) mode")
		}
	}
}
