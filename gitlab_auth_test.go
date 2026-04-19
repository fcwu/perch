package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieSignVerify(t *testing.T) {
	claims := SessionClaims{
		UserID:   "42",
		Username: "alice",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	value, err := signCookie(claims, "test-secret")
	if err != nil {
		t.Fatalf("signCookie: %v", err)
	}
	got, err := parseCookie(value, "test-secret")
	if err != nil {
		t.Fatalf("parseCookie: %v", err)
	}
	if got.UserID != claims.UserID || got.Username != claims.Username {
		t.Errorf("claims mismatch: got %+v", got)
	}
}

func TestSessionCookieTamperedSignatureRejected(t *testing.T) {
	claims := SessionClaims{UserID: "1", Username: "u", Exp: time.Now().Add(time.Hour).Unix()}
	value, _ := signCookie(claims, "secret")
	// Flip last byte of signature.
	bs := []byte(value)
	bs[len(bs)-1] ^= 0x01
	_, err := parseCookie(string(bs), "secret")
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
}

func TestSessionCookieExpiredRejected(t *testing.T) {
	claims := SessionClaims{UserID: "1", Username: "u", Exp: time.Now().Add(-time.Hour).Unix()}
	value, _ := signCookie(claims, "secret")
	_, err := parseCookie(value, "secret")
	if err == nil {
		t.Fatal("expected error for expired cookie, got nil")
	}
}

func TestGitLabAuthCallbackStateMismatch(t *testing.T) {
	ga := &gitLabAuth{
		clientID:     "cid",
		clientSecret: "csecret",
		gitlabURL:    "https://gitlab.example.com",
		redirectURI:  "https://app.example.com/auth/callback",
		cookieSecret: "test-secret",
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=wrong-state", nil)
	// Set a state cookie with a different value.
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-state"})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on state mismatch, got %d", rr.Code)
	}
}

func TestGitLabAuthCallbackTokenExchangeError(t *testing.T) {
	// Mock GitLab server that returns an error on token exchange.
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID:     "cid",
		clientSecret: "csecret",
		gitlabURL:    mockGitLab.URL,
		redirectURI:  "https://app.example.com/auth/callback",
		cookieSecret: "test-secret",
	}
	state := "valid-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=bad-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on token exchange error, got %d", rr.Code)
	}
}

func TestGitLabAuthCallbackValidFlow(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 7, "username": "bob"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID:     "cid",
		clientSecret: "csecret",
		gitlabURL:    mockGitLab.URL,
		redirectURI:  "https://app.example.com/auth/callback",
		cookieSecret: "test-secret",
	}
	state := "valid-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=good-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect (302), got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if loc != "/chat" {
		t.Errorf("expected redirect to /chat, got %q", loc)
	}
	// Verify session cookie is set.
	var sessionCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "perch_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected perch_session cookie to be set")
	}
	claims, err := parseCookie(sessionCookie.Value, "test-secret")
	if err != nil {
		t.Fatalf("parseCookie: %v", err)
	}
	if claims.UserID != "7" || claims.Username != "bob" {
		t.Errorf("unexpected claims: %+v", claims)
	}
}

func TestGitLabAuthMiddlewareMissingCookieRedirects(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret"}
	handler := ga.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect for missing cookie, got %d", rr.Code)
	}
}

func TestGitLabAuthMiddlewareTamperedCookieReturns401(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret"}
	handler := ga.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.AddCookie(&http.Cookie{Name: "perch_session", Value: "badpayload.badsig"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tampered cookie, got %d", rr.Code)
	}
}
