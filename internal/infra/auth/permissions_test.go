package auth

import (
	"sync/atomic"
	"testing"
)

func TestResolver_HasPermissionWildcard(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{"*"}, nil }
	r := NewPermissionResolver(loader)
	ok, err := r.HasPermission("u1", PermWorkersWrite)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !ok {
		t.Fatal("wildcard should grant any permission")
	}
}

func TestResolver_HasPermissionExact(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{PermWorkersRead}, nil }
	r := NewPermissionResolver(loader)
	if ok, _ := r.HasPermission("u1", PermWorkersRead); !ok {
		t.Fatal("expected workers:read granted")
	}
	if ok, _ := r.HasPermission("u1", PermWorkersWrite); ok {
		t.Fatal("expected workers:write denied")
	}
}

func TestResolver_CacheAndInvalidate(t *testing.T) {
	var calls int64
	loader := func(uid string) ([]string, error) {
		atomic.AddInt64(&calls, 1)
		return []string{PermWorkersRead}, nil
	}
	r := NewPermissionResolver(loader)
	_, _ = r.HasPermission("u1", PermWorkersRead)
	_, _ = r.HasPermission("u1", PermWorkersRead)
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("expected loader called once (cached), got %d", calls)
	}
	r.Invalidate("u1")
	_, _ = r.HasPermission("u1", PermWorkersRead)
	if atomic.LoadInt64(&calls) != 2 {
		t.Fatalf("expected loader called again after invalidate, got %d", calls)
	}
}

func TestCatalogGroupsCoverAllPermissions(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range PermissionCatalog() {
		for _, p := range g.Permissions {
			seen[p] = true
		}
	}
	for _, p := range AllPermissions() {
		if !seen[p] {
			t.Fatalf("permission %s missing from catalog groups", p)
		}
	}
}
