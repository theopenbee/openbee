package ai

import "context"

// EngineInvoker is the minimal subprocess launcher contract every engine implements.
type EngineInvoker interface {
	Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error)
}

// EngineCollector is the minimal token-usage reader contract.
type EngineCollector interface {
	Collect(ctx context.Context, sessionID string) ([]TokenUsage, error)
}

// BaseAdapter implements the EngineAdapter parts that are identical across
// engines: Run wires the invoker output into a RunResult with a bound result
// extractor; CollectTokenUsage delegates to the collector. Engines embed
// BaseAdapter and optionally override Prepare.
type BaseAdapter struct {
	Invoker   EngineInvoker
	Collector EngineCollector
	// Extract is the per-engine result extractor bound to logPath in Run.
	Extract func(logPath string) string
}

// Run launches the invoker and binds Extract to logPath in the returned RunResult.
func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
	opts RunOptions, logPath string) (RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return NewRunResult(proc, out, err, func() string { return b.Extract(logPath) })
}

// CollectTokenUsage delegates to the embedded collector.
func (b *BaseAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}

// Prepare is a no-op default that engines may override (e.g. claude).
func (b *BaseAdapter) Prepare(string, PrepareOptions) error { return nil }
