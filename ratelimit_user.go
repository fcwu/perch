package main

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// UserRateLimiter enforces per-user query rate limits (requests per minute).
// A zero RPM value disables limiting.
type UserRateLimiter struct {
	rpm int
	mu  sync.Mutex
	m   map[string]*rate.Limiter
}

func newUserRateLimiter(rpm int) *UserRateLimiter {
	return &UserRateLimiter{
		rpm: rpm,
		m:   make(map[string]*rate.Limiter),
	}
}

func (u *UserRateLimiter) limiterFor(userID string) *rate.Limiter {
	u.mu.Lock()
	defer u.mu.Unlock()
	if l, ok := u.m[userID]; ok {
		return l
	}
	l := rate.NewLimiter(rate.Every(time.Minute/time.Duration(u.rpm)), u.rpm)
	u.m[userID] = l
	return l
}

// Allow returns (true, 0) if the user is within the limit, or (false, retryAfterMs) if exceeded.
func (u *UserRateLimiter) Allow(userID string) (bool, int64) {
	if u.rpm <= 0 {
		return true, 0
	}
	l := u.limiterFor(userID)
	r := l.Reserve()
	if r.OK() && r.Delay() == 0 {
		return true, 0
	}
	// Cancel the reservation and return the wait time.
	delay := r.Delay()
	r.Cancel()
	if delay < 0 {
		delay = 0
	}
	return false, delay.Milliseconds()
}
