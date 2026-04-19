// mkcookie generates a signed perch_session cookie for testing purposes.
// Usage: go run ./cmd/mkcookie -user=42 -username=alice -role=admin [-secret=...] [-ttl=24h]
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type sessionClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role,omitempty"`
	Exp      int64  `json:"exp"`
}

func sign(claims sessionClaims, secret string) (string, error) {
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return payload + "." + fmt.Sprintf("%x", mac.Sum(nil)), nil
}

func main() {
	user := flag.String("user", "", "GitLab user ID (required)")
	username := flag.String("username", "testuser", "GitLab username")
	role := flag.String("role", "user", `role: "admin" or "user"`)
	secret := flag.String("secret", "", "COOKIE_SECRET (defaults to env COOKIE_SECRET, then perch default)")
	ttl := flag.Duration("ttl", 24*time.Hour, "cookie lifetime")
	flag.Parse()

	if *user == "" {
		fmt.Fprintln(os.Stderr, "error: -user is required")
		os.Exit(1)
	}

	s := *secret
	if s == "" {
		s = os.Getenv("COOKIE_SECRET")
	}
	if s == "" {
		s = "perch-default-secret-change-in-production"
	}

	claims := sessionClaims{
		UserID:   *user,
		Username: *username,
		Role:     *role,
		Exp:      time.Now().Add(*ttl).Unix(),
	}
	value, err := sign(claims, s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(value)
}
