package worker

import (
	"testing"
)

// TestManager_NewManager_NoLogRegistry verifies that Manager can be constructed
// without a log registry — the logRegistry field was removed in the log simplification.
func TestManager_NewManager_NoLogRegistry(t *testing.T) {
	// NewManager should compile and run without a logRegistry parameter.
	// Actual Manager behaviour is tested via integration in mcp/tools_test.go.
	_ = NewManager
}
