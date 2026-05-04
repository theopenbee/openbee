package linearcfg

import (
	"slices"
	"sync"
	"testing"
)

func TestStore_GetReturnsCopy(t *testing.T) {
	s := NewStore([]string{"a", "b"})
	got := s.Get()
	got[0] = "mutated"
	if again := s.Get(); !slices.Equal(again, []string{"a", "b"}) {
		t.Errorf("Get returned shared slice: %v", again)
	}
}

func TestStore_SetReplacesAndDropsEmpty(t *testing.T) {
	s := NewStore(nil)
	s.Set([]string{"x", "", "y"})
	if got := s.Get(); !slices.Equal(got, []string{"x", "y"}) {
		t.Errorf("expected [x y], got %v", got)
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore([]string{"seed"})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.Get() }()
		go func() { defer wg.Done(); s.Set([]string{"x"}) }()
	}
	wg.Wait()
}
