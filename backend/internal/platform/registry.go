package platform

import (
	"sort"
	"sync"
)

type Senders interface {
	Get(id string) (PlatformSenderAdapter, bool)
}

type SenderMap map[string]PlatformSenderAdapter

func (m SenderMap) Get(id string) (PlatformSenderAdapter, bool) {
	s, ok := m[id]
	return s, ok
}

type SenderRegistry struct {
	mu      sync.RWMutex
	senders map[string]PlatformSenderAdapter
}

func NewSenderRegistry() *SenderRegistry {
	return &SenderRegistry{senders: make(map[string]PlatformSenderAdapter)}
}

func (r *SenderRegistry) Get(id string) (PlatformSenderAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.senders[id]
	return s, ok
}

func (r *SenderRegistry) Set(id string, s PlatformSenderAdapter) {
	r.mu.Lock()
	r.senders[id] = s
	r.mu.Unlock()
}

func (r *SenderRegistry) Delete(id string) {
	r.mu.Lock()
	delete(r.senders, id)
	r.mu.Unlock()
}

func (r *SenderRegistry) IDs() []string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.senders))
	for id := range r.senders {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Strings(ids)
	return ids
}
