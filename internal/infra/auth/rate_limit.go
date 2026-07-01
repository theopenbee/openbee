package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LoginRateLimiter throttles login attempts per client key (typically an IP).
// It wraps a token-bucket limiter per key: up to maxAttempts in a burst, then
// one slot recovers every window/maxAttempts. Callers consume a token on each
// attempt via Allow and clear a key's budget with Reset on success, so only
// failed attempts leave a lasting cost.
type LoginRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipEntry
	rate     rate.Limit
	burst    int
	window   time.Duration
}

type ipEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	l := &LoginRateLimiter{
		limiters: make(map[string]*ipEntry),
		rate:     rate.Every(window / time.Duration(maxAttempts)),
		burst:    maxAttempts,
		window:   window,
	}
	go l.cleanupLoop()
	return l
}

// Allow consumes one token for the key and reports whether the attempt is
// permitted. Call it at the request entry point.
func (l *LoginRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.limiters[key]
	if e == nil {
		e = &ipEntry{lim: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.lim.Allow()
}

// Reset clears a key's budget so its next attempt starts from a full bucket.
// Call it after a successful login so legitimate users never accumulate a cost.
func (l *LoginRateLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.limiters, key)
	l.mu.Unlock()
}

// cleanupLoop periodically evicts keys that have been idle longer than the
// window, keeping the map from growing unboundedly with one-off client keys.
func (l *LoginRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for key, e := range l.limiters {
			if e.lastSeen.Before(cutoff) {
				delete(l.limiters, key)
			}
		}
		l.mu.Unlock()
	}
}
