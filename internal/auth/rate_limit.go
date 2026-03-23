package auth

import (
	"sync"
	"time"
)

type LoginRateLimiter struct {
	maxAttempts int
	window      time.Duration
	attempts    map[string][]time.Time
	mu          sync.Mutex
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		attempts:    make(map[string][]time.Time),
	}
}

func (l *LoginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	recent := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) == 0 {
		delete(l.attempts, ip)
	}

	if len(recent) >= l.maxAttempts {
		l.attempts[ip] = recent
		return false
	}

	l.attempts[ip] = append(recent, now)
	return true
}
