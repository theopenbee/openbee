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
