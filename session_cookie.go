package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// SessionClaims holds the data embedded in the perch_session signed cookie.
type SessionClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Exp      int64  `json:"exp"` // Unix timestamp
}

func cookieSecret() string {
	s := os.Getenv("COOKIE_SECRET")
	if s == "" {
		s = "perch-default-secret-change-in-production"
	}
	return s
}

// signCookie encodes claims as JSON, base64-encodes it, then appends an HMAC-SHA256 signature.
// Format: base64(json).<hmac-hex>
func signCookie(claims SessionClaims, secret string) (string, error) {
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := fmt.Sprintf("%x", mac.Sum(nil))
	return payload + "." + sig, nil
}

// parseCookie verifies the HMAC signature and returns the decoded claims.
func parseCookie(value, secret string) (*SessionClaims, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid cookie format")
	}
	payload, sig := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := fmt.Sprintf("%x", mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, errors.New("invalid cookie signature")
	}

	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode cookie payload: %w", err)
	}
	var claims SessionClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal cookie claims: %w", err)
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("cookie expired")
	}
	return &claims, nil
}
