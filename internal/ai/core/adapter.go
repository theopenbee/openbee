package core

import (
	"context"

	"github.com/theopenbee/openbee/internal/ai"
)

// Invoker is the minimal subprocess launcher contract every engine implements.
type Invoker interface {
	Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)
}

// Collector is the minimal token-usage reader contract.
type Collector interface {
	Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}

// Extractor reads the per-engine result text from a log file produced by Invoker.Run.
type Extractor interface {
	Extract(logPath string) string
}

// Composite is the transitional struct previously named BaseAdapter. It is
// being replaced by the BaseAdapter interface and will be removed once every
// engine migrates to core.NewEngineAdapter. New code MUST NOT reference it.
type Composite struct {
	Invoker   Invoker
	Collector Collector
	Extractor Extractor
}

// Run launches the invoker and binds the extractor to logPath in the returned RunResult.
func (b *Composite) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		return b.Extractor.Extract(logPath)
	})
}

// CollectTokenUsage delegates to the embedded collector.
func (b *Composite) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}

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
	return ai.NewRunResult(proc, out, err, func() string {
		return a.base.Extract(logPath)
	})
}

func (a *engineAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.base.Collect(ctx, sessionID)
}
