package local

import (
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
)

var log = logger.With(zap.String("component", "local"))

type SSEHub struct {
	mu          sync.Mutex
	subscribers map[string][]chan string
	seq         map[string]uint64
	closed      bool
}

func NewSSEHub() *SSEHub {
	return &SSEHub{
		subscribers: make(map[string][]chan string),
		seq:         make(map[string]uint64),
	}
}

func (h *SSEHub) Subscribe(userID string) (<-chan string, func()) {
	ch := make(chan string, 8)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subscribers[userID] = append(h.subscribers[userID], ch)
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subscribers[userID]
		for i, s := range subs {
			if s == ch {
				subs = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(subs) == 0 {
			delete(h.subscribers, userID)
			return
		}
		h.subscribers[userID] = subs
	}
}

func (h *SSEHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for userID, subs := range h.subscribers {
		for _, ch := range subs {
			close(ch)
		}
		delete(h.subscribers, userID)
	}
}

func (h *SSEHub) Publish(userID string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publishLocked(userID, ev)
}

func (h *SSEHub) PublishAll(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID := range h.subscribers {
		h.publishLocked(userID, ev)
	}
}

func (h *SSEHub) publishLocked(userID string, ev Event) {
	h.seq[userID]++
	ev.Seq = h.seq[userID]
	data, err := json.Marshal(ev)
	if err != nil {
		log.Error("sse hub: marshal event", zap.String("userID", userID), zap.Error(err))
		return
	}
	for _, ch := range h.subscribers[userID] {
		select {
		case ch <- string(data):
		default:
			log.Warn("sse hub: subscriber channel full, dropping", zap.String("userID", userID), zap.Uint64("seq", ev.Seq))
		}
	}
}
