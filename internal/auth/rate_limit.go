package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter tracks login attempts per IP address using a sliding window.
type LoginRateLimiter struct {
	maxAttempts int
	window      time.Duration
	attempts    map[string][]time.Time
	mu          sync.Mutex
}

// NewLoginRateLimiter creates a rate limiter that allows maxAttempts per window per IP.
func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		attempts:    make(map[string][]time.Time),
	}
}

// Allow returns true if the IP has not exceeded the rate limit.
func (l *LoginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Remove expired entries
	recent := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= l.maxAttempts {
		l.attempts[ip] = recent
		return false
	}

	l.attempts[ip] = append(recent, now)
	return true
}
