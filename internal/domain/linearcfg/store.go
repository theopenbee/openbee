// Package linearcfg holds the active Linear project allow-list.
//
// The list is seeded from config.yaml at startup, then optionally overridden
// by the value persisted in bee_system_configs (DB wins over yaml). The Web
// SystemSettings page mutates the DB value at runtime; the API handler calls
// Store.Set so that the running LinearReceiver picks up the change on its
// next poll tick.
package linearcfg

import "sync"

// Store holds the current Linear project allow-list. Safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	projects []string
}

// NewStore returns a Store seeded with the given project names.
func NewStore(initial []string) *Store {
	return &Store{projects: cloneNonEmpty(initial)}
}

// Get returns a copy of the current project allow-list.
func (s *Store) Get() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneNonEmpty(s.projects)
}

// Set replaces the project allow-list.
func (s *Store) Set(projects []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = cloneNonEmpty(projects)
}

func cloneNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
