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

func TestStatesStore_GetReturnsCloneOfInitial(t *testing.T) {
	s := NewStatesStore([]string{"Todo", "In Progress"})
	got := s.Get()
	if len(got) != 2 || got[0] != "Todo" || got[1] != "In Progress" {
		t.Fatalf("Get returned %v", got)
	}
	got[0] = "Mutated"
	if again := s.Get(); again[0] != "Todo" {
		t.Errorf("Get did not return a defensive copy; %v", again)
	}
}

func TestStatesStore_SetReplacesAndDropsEmpty(t *testing.T) {
	s := NewStatesStore(nil)
	s.Set([]string{"Todo", "", "In Review"})
	got := s.Get()
	if len(got) != 2 || got[0] != "Todo" || got[1] != "In Review" {
		t.Fatalf("Set did not drop empty entries: %v", got)
	}
}

func TestStatesStore_NewStoreFiltersEmptyInitial(t *testing.T) {
	s := NewStatesStore([]string{"", "Todo", ""})
	got := s.Get()
	if len(got) != 1 || got[0] != "Todo" {
		t.Errorf("NewStatesStore did not filter empty initial: %v", got)
	}
}
