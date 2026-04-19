package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGitLabAuthCallbackValidFlowSingleMode(t *testing.T) {
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
		mode:         ModeSingle,
		adminIDs:     make(map[string]bool),
		allowedIDs:   make(map[string]bool),
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
	if loc != "/" {
		t.Errorf("expected redirect to / in single mode, got %q", loc)
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

func TestGitLabAuthCallbackMultiModeAdminRedirect(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 42, "username": "admin"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID: "cid", clientSecret: "csecret",
		gitlabURL: mockGitLab.URL, redirectURI: "https://app/auth/callback",
		cookieSecret: "test-secret", mode: ModeMulti,
		adminIDs: map[string]bool{"42": true}, allowedIDs: make(map[string]bool),
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/admin" {
		t.Errorf("expected /admin redirect, got %q", loc)
	}
}

func TestGitLabAuthCallbackMultiModeAccessDenied(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 99, "username": "stranger"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID: "cid", clientSecret: "csecret",
		gitlabURL: mockGitLab.URL, redirectURI: "https://app/auth/callback",
		cookieSecret: "test-secret", mode: ModeMulti,
		adminIDs: map[string]bool{"42": true}, allowedIDs: make(map[string]bool),
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/?error=access_denied" {
		t.Errorf("expected /?error=access_denied redirect, got %q", loc)
	}
}

func TestGitLabAuthMiddlewareMissingCookieReturns401(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret"}
	handler := ga.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing cookie, got %d", rr.Code)
	}
}

func TestHandleAuthStatusNoSession(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret", adminIDs: make(map[string]bool), allowedIDs: make(map[string]bool)}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rr := httptest.NewRecorder()
	ga.handleAuthStatus(rr, req, ModeSingle, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", resp["authenticated"])
	}
	if resp["mode"] != "single" {
		t.Errorf("expected mode=single, got %v", resp["mode"])
	}
}

func TestHandleAuthStatusAdminSession(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret", adminIDs: make(map[string]bool), allowedIDs: make(map[string]bool)}
	claims := SessionClaims{UserID: "1", Username: "alice", Role: "admin", Exp: time.Now().Add(time.Hour).Unix()}
	cv, _ := signCookie(claims, "test-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "perch_session", Value: cv})
	rr := httptest.NewRecorder()
	ga.handleAuthStatus(rr, req, ModeMulti, nil)
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["authenticated"] != true {
		t.Errorf("expected authenticated=true")
	}
	if resp["role"] != "admin" {
		t.Errorf("expected role=admin, got %v", resp["role"])
	}
	if resp["mode"] != "multi" {
		t.Errorf("expected mode=multi, got %v", resp["mode"])
	}
}

func TestHandleAuthStatusUserSession(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret", adminIDs: make(map[string]bool), allowedIDs: make(map[string]bool)}
	claims := SessionClaims{UserID: "2", Username: "bob", Role: "user", Exp: time.Now().Add(time.Hour).Unix()}
	cv, _ := signCookie(claims, "test-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "perch_session", Value: cv})
	rr := httptest.NewRecorder()
	ga.handleAuthStatus(rr, req, ModeMulti, nil)
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["role"] != "user" {
		t.Errorf("expected role=user, got %v", resp["role"])
	}
}

func TestHandleLogoutClearsCookieAndRedirects(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret", adminIDs: make(map[string]bool), allowedIDs: make(map[string]bool)}
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "perch_session", Value: "somevalue"})
	rr := httptest.NewRecorder()
	ga.handleLogout(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
	var cleared bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "perch_session" && c.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected perch_session cookie to be cleared (MaxAge=-1)")
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

// AL08: GITLAB_ALLOWED_IDS=* — non-admin user redirects to /chat
func TestGitLabAuthCallbackMultiModeAllowAllRedirectsToChat(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 999, "username": "regularuser"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID: "cid", clientSecret: "csecret",
		gitlabURL: mockGitLab.URL, redirectURI: "https://app/auth/callback",
		cookieSecret: "test-secret", mode: ModeMulti,
		adminIDs: map[string]bool{"777": true}, allowedIDs: make(map[string]bool),
		allowAll: true,
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/chat" {
		t.Errorf("expected /chat redirect for allowed non-admin, got %q", loc)
	}
}

// AL16/AL21: GITLAB_ADMIN_IDS takes precedence — admin ignores GITLAB_ALLOWED_IDS restriction
func TestGitLabAuthCallbackAdminIgnoresAllowedIDs(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 42, "username": "admin"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID: "cid", clientSecret: "csecret",
		gitlabURL: mockGitLab.URL, redirectURI: "https://app/auth/callback",
		cookieSecret: "test-secret", mode: ModeMulti,
		adminIDs: map[string]bool{"42": true}, allowedIDs: map[string]bool{"99": true},
		allowAll: false,
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/admin" {
		t.Errorf("expected /admin for admin user regardless of allowedIDs, got %q", loc)
	}
}

// AL09: admin session cookie passes gitlabAuth middleware → /api/chat returns HTTP 200
func TestGitLabAuthMiddlewareAdminCookieAllowsAPIChat(t *testing.T) {
	ga := &gitLabAuth{cookieSecret: "test-secret"}
	// Forge a valid admin session cookie.
	claims := SessionClaims{
		UserID:   "42",
		Username: "admin",
		Role:     "admin",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	cv, err := signCookie(claims, "test-secret")
	if err != nil {
		t.Fatalf("signCookie: %v", err)
	}
	// Wrap a stub handler that returns 200 (simulating handleChatAPI after auth passes).
	stubChatHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := ga.middleware(stubChatHandler)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"query":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "perch_session", Value: cv})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin cookie on /api/chat, got %d", rr.Code)
	}
	// Verify context contains correct userID/username set by middleware.
	called := false
	checkCtx := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		uid, _ := r.Context().Value(ctxUserID).(string)
		uname, _ := r.Context().Value(ctxUsername).(string)
		if uid != "42" {
			t.Errorf("expected ctxUserID=42, got %q", uid)
		}
		if uname != "admin" {
			t.Errorf("expected ctxUsername=admin, got %q", uname)
		}
		w.WriteHeader(http.StatusOK)
	})
	protected2 := ga.middleware(checkCtx)
	req2 := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"query":"hi"}`))
	req2.AddCookie(&http.Cookie{Name: "perch_session", Value: cv})
	rr2 := httptest.NewRecorder()
	protected2.ServeHTTP(rr2, req2)
	if !called {
		t.Error("expected inner handler to be called for valid admin cookie")
	}
}

// AL16: single-user mode + GITLAB_ADMIN_IDS set → user not in list is rejected (HTTP 403)
func TestGitLabAuthCallbackSingleModeAdminIDsRejectsNonAdmin(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok999"})
		case "/api/v4/user":
			// Return user ID 999 — not in GITLAB_ADMIN_IDS
			json.NewEncoder(w).Encode(map[string]any{"id": 999, "username": "outsider"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID:     "cid",
		clientSecret: "csecret",
		gitlabURL:    mockGitLab.URL,
		redirectURI:  "https://app/auth/callback",
		cookieSecret: "test-secret",
		mode:         ModeSingle,
		adminIDs:     map[string]bool{"123456": true}, // only 123456 is allowed
		allowedIDs:   make(map[string]bool),
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for user not in GITLAB_ADMIN_IDS (single mode), got %d", rr.Code)
	}
	// No session cookie should have been set.
	for _, c := range rr.Result().Cookies() {
		if c.Name == "perch_session" {
			t.Error("expected no perch_session cookie for rejected user")
		}
	}
}

// AL19: user in GITLAB_ALLOWED_IDS list is permitted → /chat
func TestGitLabAuthCallbackMultiModeAllowedListPermitted(t *testing.T) {
	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"})
		case "/api/v4/user":
			json.NewEncoder(w).Encode(map[string]any{"id": 55, "username": "allowed"})
		}
	}))
	defer mockGitLab.Close()

	ga := &gitLabAuth{
		clientID: "cid", clientSecret: "csecret",
		gitlabURL: mockGitLab.URL, redirectURI: "https://app/auth/callback",
		cookieSecret: "test-secret", mode: ModeMulti,
		adminIDs: map[string]bool{"42": true}, allowedIDs: map[string]bool{"55": true},
		allowAll: false,
	}
	state := "s"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rr := httptest.NewRecorder()
	ga.handleCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/chat" {
		t.Errorf("expected /chat for listed allowed user, got %q", loc)
	}
}
