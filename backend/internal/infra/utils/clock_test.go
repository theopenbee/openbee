package utils

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextMillisIsStrictlyIncreasing(t *testing.T) {
	start := time.Now().UnixMilli()
	prev := NextMillis()
	assert.GreaterOrEqual(t, prev, start)
	for range 1000 {
		next := NextMillis()
		require.Greater(t, next, prev)
		prev = next
	}
}

func TestNextMillisIsUniqueAcrossGoroutines(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[int64]bool)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				v := NextMillis()
				mu.Lock()
				seen[v] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Len(t, seen, 1600)
}
