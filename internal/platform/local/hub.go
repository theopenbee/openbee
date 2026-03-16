package local

import (
	"log/slog"
	"sync"
)

// SSEHub manages Server-Sent Events subscriptions keyed by session key.
type SSEHub struct {
	mu          sync.Mutex
	subscribers map[string][]chan string
}

// NewSSEHub constructs an SSEHub.
func NewSSEHub() *SSEHub {
	return &SSEHub{subscribers: make(map[string][]chan string)}
}

// Subscribe registers a new SSE client for the given session key.
// Returns the receive channel and an unsubscribe function the caller must invoke on disconnect.
func (h *SSEHub) Subscribe(sessionKey string) (<-chan string, func()) {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.subscribers[sessionKey] = append(h.subscribers[sessionKey], ch)
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subscribers[sessionKey]
		for i, s := range subs {
			if s == ch {
				h.subscribers[sessionKey] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}
}

// Broadcast sends data to all subscribers of the given session key.
// If a subscriber's channel is full, it is dropped with a warning.
func (h *SSEHub) Broadcast(sessionKey, data string) {
	h.mu.Lock()
	subs := make([]chan string, len(h.subscribers[sessionKey]))
	copy(subs, h.subscribers[sessionKey])
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- data:
		default:
			slog.Warn("sse hub: subscriber channel full, dropping", "sessionKey", sessionKey)
		}
	}
}
