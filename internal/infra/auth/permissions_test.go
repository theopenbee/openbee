package auth

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolver_HasPermissionWildcard(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{"*"}, nil }
	r := NewPermissionResolver(loader)
	ok, err := r.HasPermission("u1", PermContactsWrite)
	require.NoError(t, err)
	require.True(t, ok, "wildcard should grant any permission")
}

func TestResolver_HasPermissionExact(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{PermContactsRead}, nil }
	r := NewPermissionResolver(loader)
	ok, _ := r.HasPermission("u1", PermContactsRead)
	require.True(t, ok, "expected contacts:read granted")
	ok, _ = r.HasPermission("u1", PermContactsWrite)
	require.False(t, ok, "expected contacts:write denied")
}

func TestResolver_HasAnyPermission(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{PermUsersManage}, nil }
	r := NewPermissionResolver(loader)

	// Holds one of the listed perms -> granted.
	ok, _ := r.HasAnyPermission("u1", PermRolesManage, PermUsersManage)
	require.True(t, ok, "expected any-of {roles:manage, users:manage} granted via users:manage")
	// Holds none of the listed perms -> denied.
	ok, _ = r.HasAnyPermission("u1", PermRolesManage, PermContactsWrite)
	require.False(t, ok, "expected any-of {roles:manage, contacts:write} denied")
	// No perms supplied -> denied.
	ok, _ = r.HasAnyPermission("u1")
	require.False(t, ok, "expected no-perms any-of to be denied")
}

func TestResolver_HasAnyPermissionWildcard(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{"*"}, nil }
	r := NewPermissionResolver(loader)
	ok, _ := r.HasAnyPermission("u1", PermRolesManage)
	require.True(t, ok, "wildcard should satisfy any-of")
}

func TestResolver_CacheAndInvalidate(t *testing.T) {
	var calls int64
	loader := func(uid string) ([]string, error) {
		atomic.AddInt64(&calls, 1)
		return []string{PermContactsRead}, nil
	}
	r := NewPermissionResolver(loader)
	_, _ = r.HasPermission("u1", PermContactsRead)
	_, _ = r.HasPermission("u1", PermContactsRead)
	require.EqualValues(t, 1, atomic.LoadInt64(&calls))
	r.Invalidate("u1")
	_, _ = r.HasPermission("u1", PermContactsRead)
	require.EqualValues(t, 2, atomic.LoadInt64(&calls))
}

func TestCatalogGroupsCoverAllPermissions(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range PermissionCatalog() {
		for _, p := range g.Permissions {
			seen[p] = true
		}
	}
	for _, p := range AllPermissions() {
		require.True(t, seen[p], "permission %s missing from catalog groups", p)
	}
}

// messages:read was merged into chat:write; it must no longer be assignable.
func TestMessagesReadRetired(t *testing.T) {
	require.False(t, IsAssignablePermission("messages:read"), "messages:read should be retired (merged into chat:write)")
}
