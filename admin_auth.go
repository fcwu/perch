package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type adminAuth struct {
	token        string
	cookieSecret string
}

func newAdminAuth() *adminAuth {
	return &adminAuth{
		token:        os.Getenv("ADMIN_TOKEN"),
		cookieSecret: cookieSecret(),
	}
}

func (a *adminAuth) enabled() bool { return a.token != "" }

// handleLogin handles POST /admin/login — validates ADMIN_TOKEN, sets perch_admin cookie.
func (a *adminAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.enabled() {
		http.Error(w, "admin not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Token != a.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	claims := SessionClaims{
		UserID:   "admin",
		Username: "admin",
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	}
	cookieValue, err := signCookie(claims, a.cookieSecret)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "perch_admin",
		Value:    cookieValue,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   int(24 * time.Hour / time.Second),
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
}

// isAdmin returns true if the request carries a valid admin cookie.
func (a *adminAuth) isAdmin(r *http.Request) bool {
	if !a.enabled() {
		return false
	}
	cookie, err := r.Cookie("perch_admin")
	if err != nil {
		return false
	}
	_, err = parseCookie(cookie.Value, a.cookieSecret)
	return err == nil
}

type contextAdminKey struct{}

// middleware protects /admin/* routes (except /admin/login).
// If ADMIN_TOKEN is not configured, all requests are allowed through.
// Returns 401 if cookie is missing/invalid when token is configured.
func (a *adminAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.enabled() {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("perch_admin")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := parseCookie(cookie.Value, a.cookieSecret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), contextAdminKey{}, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
