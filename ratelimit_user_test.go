package main

import (
	"testing"
)

func TestUserRateLimiterWithinLimit(t *testing.T) {
	rl := newUserRateLimiter(10)
	for i := 0; i < 10; i++ {
		ok, _ := rl.Allow("alice")
		if !ok {
			t.Errorf("request %d should be allowed within limit", i+1)
		}
	}
}

func TestUserRateLimiterExceedLimit(t *testing.T) {
	rl := newUserRateLimiter(2)
	// Consume all tokens
	rl.Allow("alice")
	rl.Allow("alice")
	// Next request should be denied
	ok, retryAfter := rl.Allow("alice")
	if ok {
		t.Error("request should be denied after exceeding limit")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retry_after_ms, got %d", retryAfter)
	}
}

func TestUserRateLimiterDisabled(t *testing.T) {
	rl := newUserRateLimiter(0)
	for i := 0; i < 100; i++ {
		ok, _ := rl.Allow("alice")
		if !ok {
			t.Errorf("request %d should be allowed when RPM=0 (disabled)", i+1)
		}
	}
}

func TestUserRateLimiterIsolation(t *testing.T) {
	rl := newUserRateLimiter(1)
	// alice uses her quota
	rl.Allow("alice")
	// alice is now rate limited
	okAlice, _ := rl.Allow("alice")
	if okAlice {
		t.Error("alice should be rate limited")
	}
	// bob is not affected
	okBob, _ := rl.Allow("bob")
	if !okBob {
		t.Error("bob should not be rate limited by alice's usage")
	}
}
