package worker

import (
	"strings"
	"sync"

	"go.uber.org/zap"
)

// executionLog is a thread-safe log buffer for in-flight executions.
type executionLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *executionLog) writeLine(s string) {
	l.mu.Lock()
	l.buf.WriteString(s)
	l.buf.WriteByte('\n')
	l.mu.Unlock()
}

func (l *executionLog) string() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// ActiveLogRegistry manages live log buffers for all running executions.
// Both Manager (worker executions) and Feeder (bee executions) write to it.
// It is safe for concurrent use by multiple goroutines.
type ActiveLogRegistry struct {
	mu   sync.RWMutex
	logs map[string]*executionLog
}

// NewActiveLogRegistry returns an empty registry.
func NewActiveLogRegistry() *ActiveLogRegistry {
	return &ActiveLogRegistry{logs: make(map[string]*executionLog)}
}

// Register creates a new log buffer for the given execution ID and returns
// a function that callers use to append lines.
// If id is already registered (a bug), Register logs a warning and returns
// a no-op function — it does NOT panic.
func (r *ActiveLogRegistry) Register(id string) func(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.logs[id]; exists {
		log.Warn("ActiveLogRegistry.Register: id already registered, returning no-op",
			zap.String("execution_id", id))
		return func(string) {}
	}

	el := &executionLog{}
	r.logs[id] = el
	return el.writeLine
}

// Get returns the current accumulated content for id.
// Returns ("", false) if no active buffer exists for that id.
func (r *ActiveLogRegistry) Get(id string) (string, bool) {
	r.mu.RLock()
	el, ok := r.logs[id]
	r.mu.RUnlock()
	if !ok {
		return "", false
	}
	return el.string(), true
}

// Unregister removes the log buffer for id.
// Safe to call even if id is absent.
func (r *ActiveLogRegistry) Unregister(id string) {
	r.mu.Lock()
	delete(r.logs, id)
	r.mu.Unlock()
}
