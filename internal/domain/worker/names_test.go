package worker

import (
	"testing"
)

func TestPickRandomName_AllUnused(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{}
	name, ok := PickRandomName(pool, used)
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	found := false
	for _, p := range pool {
		if p == name {
			found = true
		}
	}
	if !found {
		t.Errorf("returned name %q not in pool", name)
	}
}

func TestPickRandomName_SomeUsed(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	name, ok := PickRandomName(pool, used)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Carol" {
		t.Errorf("expected Carol, got %q", name)
	}
}

func TestPickRandomName_AllUsed(t *testing.T) {
	pool := []string{"Alice", "Bob"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	_, ok := PickRandomName(pool, used)
	if ok {
		t.Fatal("expected ok=false when all names used")
	}
}

func TestPickRandomName_CaseInsensitive(t *testing.T) {
	pool := []string{"Alice"}
	used := map[string]struct{}{"ALICE": {}}
	_, ok := PickRandomName(pool, used)
	if ok {
		t.Fatal("expected ok=false: pool name should be filtered case-insensitively")
	}
}

func TestNamePool_ZH(t *testing.T) {
	pool := NamePool("zh")
	if len(pool) != 200 {
		t.Errorf("zh pool: want 200 names, got %d", len(pool))
	}
}

func TestNamePool_EN(t *testing.T) {
	pool := NamePool("en")
	if len(pool) != 200 {
		t.Errorf("en pool: want 200 names, got %d", len(pool))
	}
}

func TestNamePool_DefaultsToEN(t *testing.T) {
	pool := NamePool("fr")
	if len(pool) != 200 {
		t.Errorf("unknown lang: want 200-name EN pool, got %d", len(pool))
	}
}
