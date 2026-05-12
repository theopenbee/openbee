package pi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.ExtraEnv())
	})
}

type piAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, extraEnv)
	if err != nil {
		return nil, err
	}
	return &piAdapter{invoker: inv, collector: NewCollector()}, nil
}

func (a *piAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *piAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

func (a *piAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
