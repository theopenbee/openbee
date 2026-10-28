package enginecfg_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestInit(t *testing.T) {
	s := enginecfg.NewStore("claude")
	assert.Equal(t, "claude", s.Get())
}

func TestSet(t *testing.T) {
	s := enginecfg.NewStore("claude")
	s.Set("codex")
	assert.Equal(t, "codex", s.Get())
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
