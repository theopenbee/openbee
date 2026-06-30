package auth

import "sync"

// PermWildcard grants every permission (super-admin).
const PermWildcard = "*"

// Permission keys ("resource:action"). Single source of truth.
const (
	PermContactsRead      = "contacts:read"
	PermContactsWrite     = "contacts:write"
	PermTasksRead         = "tasks:read"
	PermTasksWrite        = "tasks:write"
	PermChatWrite         = "chat:write"
	PermSessionsRead      = "sessions:read"
	PermSessionsWrite     = "sessions:write"
	PermDashboardRead     = "dashboard:read"
	PermEnvRead           = "env:read"
	PermEnvWrite          = "env:write"
	PermSystemConfigRead  = "system_config:read"
	PermSystemConfigWrite = "system_config:write"
	PermUsersManage       = "users:manage"
	PermRolesManage       = "roles:manage"
)

// PermissionGroup groups permissions by resource for the catalog endpoint.
type PermissionGroup struct {
	Resource    string   `json:"resource"`
	Permissions []string `json:"permissions"`
}

// PermissionCatalog returns the grouped permission catalog for the UI.
func PermissionCatalog() []PermissionGroup {
	return []PermissionGroup{
		{Resource: "contacts", Permissions: []string{PermContactsRead, PermContactsWrite}},
		{Resource: "chat", Permissions: []string{PermChatWrite}},
		{Resource: "tasks", Permissions: []string{PermTasksRead, PermTasksWrite}},
		{Resource: "sessions", Permissions: []string{PermSessionsRead, PermSessionsWrite}},
		{Resource: "dashboard", Permissions: []string{PermDashboardRead}},
		{Resource: "env", Permissions: []string{PermEnvRead, PermEnvWrite}},
		{Resource: "system_config", Permissions: []string{PermSystemConfigRead, PermSystemConfigWrite}},
		{Resource: "administration", Permissions: []string{PermUsersManage, PermRolesManage}},
	}
}

// AllPermissions flattens the catalog into a single slice.
func AllPermissions() []string {
	var out []string
	for _, g := range PermissionCatalog() {
		out = append(out, g.Permissions...)
	}
	return out
}

// assignablePermissions is the set of known, assignable permission keys
// (wildcard excluded), built once from the immutable catalog.
var assignablePermissions = func() map[string]struct{} {
	perms := AllPermissions()
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return set
}()

// IsAssignablePermission reports whether p is a known, assignable permission.
func IsAssignablePermission(p string) bool {
	_, ok := assignablePermissions[p]
	return ok
}

// PermissionLoader loads the raw permission keys for a user (union across roles).
type PermissionLoader func(userID string) ([]string, error)

// PermissionResolver caches per-user permission sets and answers HasPermission.
type PermissionResolver struct {
	load  PermissionLoader
	mu    sync.RWMutex
	cache map[string]map[string]struct{}
}

func NewPermissionResolver(loader PermissionLoader) *PermissionResolver {
	return &PermissionResolver{load: loader, cache: map[string]map[string]struct{}{}}
}

func (r *PermissionResolver) permSet(userID string) (map[string]struct{}, error) {
	r.mu.RLock()
	if set, ok := r.cache[userID]; ok {
		r.mu.RUnlock()
		return set, nil
	}
	r.mu.RUnlock()

	perms, err := r.load(userID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	r.mu.Lock()
	r.cache[userID] = set
	r.mu.Unlock()
	return set, nil
}

// HasPermission reports whether the user holds perm (or the wildcard).
func (r *PermissionResolver) HasPermission(userID, perm string) (bool, error) {
	set, err := r.permSet(userID)
	if err != nil {
		return false, err
	}
	if _, ok := set[PermWildcard]; ok {
		return true, nil
	}
	_, ok := set[perm]
	return ok, nil
}

// PermissionsFor returns the user's resolved permission keys (for /api/me).
func (r *PermissionResolver) PermissionsFor(userID string) ([]string, error) {
	set, err := r.permSet(userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out, nil
}

// Invalidate drops the cached set for one user.
func (r *PermissionResolver) Invalidate(userID string) {
	r.mu.Lock()
	delete(r.cache, userID)
	r.mu.Unlock()
}

// InvalidateAll clears the entire cache (use when role permissions change).
func (r *PermissionResolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = map[string]map[string]struct{}{}
	r.mu.Unlock()
}
