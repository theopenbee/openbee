// Package enginecfg holds the process-wide default engine name.
// Updated at startup from DB/config and whenever the /engine command fires.
package enginecfg

import "sync"

var (
	mu  sync.RWMutex
	val string
)

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
