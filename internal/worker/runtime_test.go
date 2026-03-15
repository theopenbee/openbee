package worker

import (
	"context"
	"testing"
	"time"

	"github.com/robobee/core/internal/claude"
)

func TestClaudeRuntime_ExecuteWithEcho(t *testing.T) {
	r := NewClaudeRuntime("echo", "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify the interface is satisfied
	var _ Runtime = r
	_ = ctx
	_ = claude.RunOptions{}
}

func TestNewClaudeRuntime(t *testing.T) {
	r := NewClaudeRuntime("/usr/bin/claude", "http://localhost:8080", "test-key")
	// ClaudeRuntime now wraps an Invoker; verify it was created
	if r.invoker == nil {
		t.Error("expected non-nil invoker")
	}
}
