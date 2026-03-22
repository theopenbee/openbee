package weixin

import (
	"testing"
	"time"
)

func TestSessionManager_NotPausedByDefault(t *testing.T) {
	sm := newSessionManager()
	if sm.isPaused() {
		t.Error("session should not be paused by default")
	}
}

func TestSessionManager_PauseAndResume(t *testing.T) {
	sm := &sessionManager{pauseDuration: 50 * time.Millisecond}
	sm.pause()
	if !sm.isPaused() {
		t.Error("session should be paused after pause()")
	}
	remaining := sm.remainingPause()
	if remaining <= 0 || remaining > 50*time.Millisecond {
		t.Errorf("remaining = %v, want (0, 50ms]", remaining)
	}
	time.Sleep(60 * time.Millisecond)
	if sm.isPaused() {
		t.Error("session should auto-resume after pause duration")
	}
}

func TestSessionManager_RemainingWhenNotPaused(t *testing.T) {
	sm := newSessionManager()
	if sm.remainingPause() != 0 {
		t.Error("remaining should be 0 when not paused")
	}
}
