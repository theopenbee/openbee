// Package enginecfg holds the process-wide default engine name.
// It is initialized once at startup and updated when the /engine command fires.
package enginecfg

import "sync"

var (
	mu  sync.RWMutex
	val string
)

// Init sets the initial engine name. Call once at app startup.
func Init(engine string) {
	mu.Lock()
	defer mu.Unlock()
	val = engine
}

// Get returns the current default engine name.
func Get() string {
	mu.RLock()
	defer mu.RUnlock()
	return val
}

// Set updates the default engine name. Safe for concurrent use.
func Set(engine string) {
	mu.Lock()
	defer mu.Unlock()
	val = engine
}
