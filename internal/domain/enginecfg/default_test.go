package enginecfg_test

import (
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestInit(t *testing.T) {
	s := enginecfg.NewStore("claude")
	if got := s.Get(); got != "claude" {
		t.Errorf("Init: expected claude, got %s", got)
	}
}

func TestSet(t *testing.T) {
	s := enginecfg.NewStore("claude")
	s.Set("codex")
	if got := s.Get(); got != "codex" {
		t.Errorf("Set: expected codex, got %s", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := enginecfg.NewStore("claude")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Set("codex") }()
		go func() { defer wg.Done(); _ = s.Get() }()
	}
	wg.Wait()
	// No race condition — test passes if race detector doesn't fire.
}
