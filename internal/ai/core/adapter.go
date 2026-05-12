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

// BaseAdapter implements the EngineAdapter parts that are identical across
// engines: Run wires the invoker output into a RunResult with a bound result
// extractor; CollectTokenUsage delegates to the collector. Engines embed
// BaseAdapter and optionally override Run.
type BaseAdapter struct {
	Invoker   Invoker
	Collector Collector
	// Extractor is the per-engine result extractor; preferred over Extract when set.
	Extractor Extractor
	// Extract is the legacy func-typed extractor; will be removed once all engines migrate.
	Extract func(logPath string) string
}

// Run launches the invoker and binds the extractor to logPath in the returned RunResult.
func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		if b.Extractor != nil {
			return b.Extractor.Extract(logPath)
		}
		return b.Extract(logPath)
	})
}

// CollectTokenUsage delegates to the embedded collector.
func (b *BaseAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}
