// Package linearcfg holds runtime-mutable Linear allow-lists (project keys,
// workflow-state names). Each list is seeded from config.yaml at startup,
// then optionally overridden by the value persisted in bee_system_configs
// (DB wins over yaml). The Web SystemSettings page mutates the DB value at
// runtime; the API handler calls Store.Set so that the running LinearReceiver
// picks up the change on its next poll tick.
package linearcfg

import "sync"

// Store holds a string allow-list. Safe for concurrent use.
type Store struct {
	mu     sync.RWMutex
	values []string
}

// NewStore returns a Store seeded with the given values. Empty entries are
// dropped.
func NewStore(initial []string) *Store {
	return &Store{values: cloneNonEmpty(initial)}
}

// Get returns a copy of the current allow-list.
func (s *Store) Get() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneNonEmpty(s.values)
}

// Set replaces the allow-list. Empty entries are dropped.
func (s *Store) Set(values []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = cloneNonEmpty(values)
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
