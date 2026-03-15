// internal/worker/claude_runtime.go
package worker

import (
	"context"
	"sync"

	"github.com/robobee/core/internal/claude"
)

type ClaudeRuntime struct {
	invoker *claude.Invoker
	proc    *claude.Process
	mu      sync.Mutex
}

func NewClaudeRuntime(binary, mcpBaseURL, apiKey string) *ClaudeRuntime {
	return &ClaudeRuntime{
		invoker: claude.NewInvoker(binary, mcpBaseURL, apiKey),
	}
}

func (r *ClaudeRuntime) Execute(ctx context.Context, workDir string, plan string, opts claude.RunOptions) (<-chan claude.Output, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, ch, err := r.invoker.Run(ctx, workDir, plan, opts)
	if err != nil {
		return nil, err
	}
	r.proc = proc
	return ch, nil
}

func (r *ClaudeRuntime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proc != nil {
		return r.proc.Stop()
	}
	return nil
}

func (r *ClaudeRuntime) PID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proc != nil {
		return r.proc.PID()
	}
	return 0
}
