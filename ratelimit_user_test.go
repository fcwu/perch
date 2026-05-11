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

func TestUserRateLimiterSetRPMResetsExistingLimiters(t *testing.T) {
	rl := newUserRateLimiter(2)
	// Consume all tokens for alice.
	rl.Allow("alice")
	rl.Allow("alice")
	if ok, _ := rl.Allow("alice"); ok {
		t.Fatal("alice should be limited after burning her bucket")
	}
	// Bump RPM to 10 — existing per-user limiter must be discarded so the
	// new rate takes effect immediately, not after the old limiter refills.
	rl.SetRPM(10)
	for i := 0; i < 10; i++ {
		if ok, _ := rl.Allow("alice"); !ok {
			t.Errorf("request %d should be allowed after SetRPM(10) reset", i+1)
		}
	}
}

func TestUserRateLimiterSetRPMToZeroDisables(t *testing.T) {
	rl := newUserRateLimiter(1)
	rl.Allow("alice")
	if ok, _ := rl.Allow("alice"); ok {
		t.Fatal("alice should be limited at rpm=1")
	}
	rl.SetRPM(0)
	for i := 0; i < 5; i++ {
		if ok, _ := rl.Allow("alice"); !ok {
			t.Errorf("expected unlimited after SetRPM(0), but request %d denied", i+1)
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
