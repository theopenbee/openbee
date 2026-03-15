// internal/worker/runtime.go
package worker

import (
	"context"

	"github.com/robobee/core/internal/claude"
)

type Runtime interface {
	Execute(ctx context.Context, workDir string, plan string, opts claude.RunOptions) (<-chan claude.Output, error)
	PID() int
	Stop() error
}
