// Package enginecfg holds the active default engine name.
// A Store instance is created at startup (seeded from config/DB) and injected
// into every subsystem that needs to observe or mutate the default engine.
package enginecfg

import "sync"

// Store holds the current default engine name. Safe for concurrent use.
type Store struct {
	mu  sync.RWMutex
	val string
}

// NewStore returns a Store seeded with the given engine name.
func NewStore(initial string) *Store {
	return &Store{val: initial}
}

// Get returns the current default engine name.
func (s *Store) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.val
}

// Set updates the default engine name.
func (s *Store) Set(engine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val = engine
}
