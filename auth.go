package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
)

type AuthMiddleware struct {
	mode     string
	password string
	mu       sync.Mutex
	sessions map[string]struct{}
}

func newAuthMiddleware(mode, password string) *AuthMiddleware {
	return &AuthMiddleware{
		mode:     mode,
		password: password,
		sessions: make(map[string]struct{}),
	}
}

func (a *AuthMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch a.mode {
		case "none":
			next.ServeHTTP(w, r)
		case "password":
			if r.URL.Path == "/login" {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie("session")
			if err != nil || !a.validSession(cookie.Value) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, "local")
			ctx = context.WithValue(ctx, ctxUsername, "local")
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

func (a *AuthMiddleware) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct{ Password string `json:"password"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password != a.password {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	token := a.newSession()
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: token,
		HttpOnly: true, Path: "/",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthMiddleware) newSession() string {
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)
	a.mu.Lock()
	a.sessions[token] = struct{}{}
	a.mu.Unlock()
	return token
}

func (a *AuthMiddleware) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil || !a.validSession(cookie.Value) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthMiddleware) validSession(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.sessions[token]
	return ok
}

// CheckSession returns true if the request has a valid password session cookie.
func (a *AuthMiddleware) CheckSession(r *http.Request) bool {
	if a == nil || a.mode != "password" {
		return false
	}
	cookie, err := r.Cookie("session")
	return err == nil && a.validSession(cookie.Value)
}
