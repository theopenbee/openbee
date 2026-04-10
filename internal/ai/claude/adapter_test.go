package claude_test

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", "http://localhost:9999")
}

func TestClaudeAdapter_Prepare_Stub(t *testing.T) {
	// Placeholder — replaced in Task 3 with real cleanup tests.
	dir := t.TempDir()
	adapter := newTestAdapter(t)
	if err := adapter.Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}
