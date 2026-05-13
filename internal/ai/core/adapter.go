package core

import (
	"context"

	"github.com/theopenbee/openbee/internal/ai"
)

// BaseAdapter is the unified engine-side contract. Engine packages provide a
// single type (typically named Backend) satisfying this interface.
type BaseAdapter interface {
	// Run launches the engine subprocess; the returned channel is closed after
	// the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)

	// Collect reads per-turn token usage for the given session. Returns
	// ai.ErrSessionDataNotFound when no data is available.
	Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)

	// Extract reads the per-engine result text from the log file produced by Run.
	Extract(logPath string) string
}

// NewEngineAdapter wraps a BaseAdapter to satisfy ai.EngineAdapter. The
// wrapper binds Extract to logPath in the returned ai.RunResult and delegates
// CollectTokenUsage to Collect.
func NewEngineAdapter(base BaseAdapter) ai.EngineAdapter {
	return &engineAdapter{base: base}
}

type engineAdapter struct{ base BaseAdapter }

func (a *engineAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.base.Run(ctx, workDir, prompt, opts, logPath)
	return NewRunResult(proc, out, err, func() string {
		return a.base.Extract(logPath)
	})
}

func (a *engineAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.base.Collect(ctx, sessionID)
}
