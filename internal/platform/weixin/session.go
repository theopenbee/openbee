package weixin

import (
	"sync"
	"time"
)

const defaultPauseDuration = 1 * time.Hour

type sessionManager struct {
	mu            sync.Mutex
	pausedAt      time.Time
	pauseDuration time.Duration
}

func newSessionManager() *sessionManager {
	return &sessionManager{pauseDuration: defaultPauseDuration}
}

func (s *sessionManager) pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pausedAt = time.Now()
}

func (s *sessionManager) isPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pausedAt.IsZero() {
		return false
	}
	return time.Since(s.pausedAt) < s.pauseDuration
}

func (s *sessionManager) remainingPause() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pausedAt.IsZero() {
		return 0
	}
	remaining := s.pauseDuration - time.Since(s.pausedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
