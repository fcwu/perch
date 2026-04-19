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
}

func newGitLabAuth() *gitLabAuth {
	return &gitLabAuth{
		clientID:     os.Getenv("GITLAB_CLIENT_ID"),
		clientSecret: os.Getenv("GITLAB_CLIENT_SECRET"),
		redirectURI:  os.Getenv("GITLAB_REDIRECT_URI"),
		gitlabURL:    strings.TrimRight(os.Getenv("GITLAB_URL"), "/"),
		cookieSecret: cookieSecret(),
	}
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

	claims := SessionClaims{
		UserID:   fmt.Sprintf("%d", profile.ID),
		Username: profile.Username,
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
	http.Redirect(w, r, "/chat", http.StatusFound)
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
// Invalid/missing cookie → redirect to /auth/gitlab. Tampered cookie → 401.
func (g *gitLabAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("perch_session")
		if err != nil {
			http.Redirect(w, r, "/auth/gitlab", http.StatusFound)
			return
		}
		claims, err := parseCookie(cookie.Value, g.cookieSecret)
		if err != nil {
			if err.Error() == "invalid cookie signature" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// Expired or missing → redirect to login
			http.Redirect(w, r, "/auth/gitlab", http.StatusFound)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxUsername, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
