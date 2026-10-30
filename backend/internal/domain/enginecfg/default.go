package enginecfg

import (
	"slices"
	"sync"
)

type Store struct {
	mu            sync.RWMutex
	selected      string
	configDefault string
	enabled       func() []string
}

func NewStore(selected, configDefault string, enabled func() []string) *Store {
	return &Store{selected: selected, configDefault: configDefault, enabled: enabled}
}

func (s *Store) Get() string {
	ids := s.enabled()
	if len(ids) == 0 {
		return ""
	}
	selected := s.Selected()
	if slices.Contains(ids, selected) {
		return selected
	}
	if slices.Contains(ids, s.configDefault) {
		return s.configDefault
	}
	return slices.Min(ids)
}

func (s *Store) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected
}

func (s *Store) Set(engine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = engine
}

func (s *Store) Resolve(workerEngine string) string {
	if workerEngine != "" {
		return workerEngine
	}
	return s.Get()
}
