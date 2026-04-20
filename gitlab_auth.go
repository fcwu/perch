package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type gitLabAuth struct {
	clientID     string
	clientSecret string
	redirectURI  string
	gitlabURL    string
	cookieSecret string
	mode         OperatingMode
	authMethod   string          // AUTH_METHOD value: none | password | mtls | gitlab
	adminIDs     map[string]bool // GitLab user IDs that get admin role
	allowedIDs   map[string]bool // non-admin IDs allowed in multi-user mode; nil = deny all; "∗" sentinel = allow all
	allowAll     bool            // true when GITLAB_ALLOWED_IDS=*
}

func newGitLabAuth(mode OperatingMode, authMethod string) *gitLabAuth {
	g := &gitLabAuth{
		clientID:     os.Getenv("GITLAB_CLIENT_ID"),
		clientSecret: os.Getenv("GITLAB_CLIENT_SECRET"),
		redirectURI:  os.Getenv("GITLAB_REDIRECT_URI"),
		gitlabURL:    strings.TrimRight(os.Getenv("GITLAB_URL"), "/"),
		cookieSecret: cookieSecret(),
		mode:         mode,
		authMethod:   authMethod,
		adminIDs:     make(map[string]bool),
		allowedIDs:   make(map[string]bool),
	}
	for _, id := range splitIDs(os.Getenv("GITLAB_ADMIN_IDS")) {
		g.adminIDs[id] = true
	}
	allowed := os.Getenv("GITLAB_ALLOWED_IDS")
	if allowed == "*" {
		g.allowAll = true
	} else {
		for _, id := range splitIDs(allowed) {
			g.allowedIDs[id] = true
		}
	}
	return g
}

func splitIDs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (g *gitLabAuth) enabled() bool {
	return g.clientID != "" && g.clientSecret != "" && g.gitlabURL != ""
}

// randomState generates a cryptographically random state token.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// requestRedirectURI builds the redirect URI from the incoming request's host,
// falling back to the configured GITLAB_REDIRECT_URI env var if the request
// host is empty. This lets the OAuth flow work regardless of which hostname
// the user accessed (e.g. localhost vs the public FQDN).
func (g *gitLabAuth) requestRedirectURI(r *http.Request) string {
	host := r.Host
	if host == "" {
		return g.redirectURI
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + host + "/auth/callback"
}

// handleRedirect redirects the user to GitLab's OAuth consent page.
func (g *gitLabAuth) handleRedirect(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	redirectURI := g.requestRedirectURI(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Path:     "/auth",
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})
	// Store the redirect URI so the callback can use the same value for token exchange.
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_redirect_uri",
		Value:    redirectURI,
		HttpOnly: true,
		Path:     "/auth",
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})
	params := url.Values{
		"client_id":     {g.clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"read_user"},
		"state":         {state},
	}
	authURL := g.gitlabURL + "/oauth/authorize?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback handles the OAuth callback: exchanges code for token, fetches user profile, sets session cookie.
func (g *gitLabAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		MaxAge: -1,
		Path:   "/auth",
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	// Use the redirect URI that was recorded when the flow started.
	redirectURI := g.redirectURI
	if c, err2 := r.Cookie("oauth_redirect_uri"); err2 == nil && c.Value != "" {
		redirectURI = c.Value
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_redirect_uri",
		Value:  "",
		MaxAge: -1,
		Path:   "/auth",
	})

	token, err := g.exchangeCode(r.Context(), code, redirectURI)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	profile, err := g.fetchProfile(r.Context(), token)
	if err != nil {
		http.Error(w, "profile fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	userID := fmt.Sprintf("%d", profile.ID)
	role := ""
	redirect := "/"

	if g.mode == ModeMulti {
		// Multi-user mode: admin check first, then allowedIDs.
		if g.adminIDs[userID] {
			role = "admin"
			redirect = "/admin"
		} else if g.allowAll || g.allowedIDs[userID] {
			role = "user"
			redirect = "/chat"
		} else {
			http.Redirect(w, r, "/?error=access_denied", http.StatusFound)
			return
		}
	} else {
		// Single-user GitLab mode: adminIDs as allowlist (empty = allow all).
		if len(g.adminIDs) > 0 && !g.adminIDs[userID] {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		role = "admin"
		redirect = "/"
	}

	claims := SessionClaims{
		UserID:   userID,
		Username: profile.Username,
		Role:     role,
		Exp:      time.Now().Add(8 * time.Hour).Unix(),
	}
	cookieValue, err := signCookie(claims, g.cookieSecret)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "perch_session",
		Value:    cookieValue,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(8 * time.Hour / time.Second),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, redirect, http.StatusFound)
}

// handleLogout clears session cookies and redirects to /.
func (g *gitLabAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "perch_session", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleAuthStatus returns the current auth state as JSON (always HTTP 200).
func (g *gitLabAuth) handleAuthStatus(w http.ResponseWriter, r *http.Request, mode OperatingMode, auth *AuthMiddleware) {
	type statusResponse struct {
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
		Role          string `json:"role"`
		Mode          string `json:"mode"`
		AuthMethod    string `json:"auth_method"`
	}
	resp := statusResponse{Mode: string(mode), AuthMethod: g.authMethod}
	if g.authMethod == "none" {
		resp.Authenticated = true
	}
	// Check GitLab session cookie.
	if !resp.Authenticated {
		if cookie, err := r.Cookie("perch_session"); err == nil {
			if claims, err := parseCookie(cookie.Value, g.cookieSecret); err == nil {
				resp.Authenticated = true
				resp.Username = claims.Username
				resp.Role = claims.Role
			}
		}
	}
	// Check password session cookie.
	if !resp.Authenticated && auth.CheckSession(r) {
		resp.Authenticated = true
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type gitLabTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func (g *gitLabAuth) exchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	params := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.gitlabURL+"/oauth/token", strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tr gitLabTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("gitlab error: %s", tr.Error)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}
	return tr.AccessToken, nil
}

type gitLabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (g *gitLabAuth) fetchProfile(ctx context.Context, token string) (*gitLabUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.gitlabURL+"/api/v4/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var u gitLabUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parse user response: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("invalid user profile")
	}
	return &u, nil
}

type contextKey string

const (
	ctxUserID   contextKey = "userID"
	ctxUsername contextKey = "username"
)

// middleware protects routes requiring a valid GitLab session cookie.
// Missing/invalid/expired session → HTTP 401 JSON (not redirect).
func (g *gitLabAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("perch_session")
		if err != nil {
			g.unauthorized(w)
			return
		}
		claims, err := parseCookie(cookie.Value, g.cookieSecret)
		if err != nil {
			g.unauthorized(w)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxUsername, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *gitLabAuth) unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
