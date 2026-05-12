package kimi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineKimi), cfg.ExtraEnv()), nil
	})
}

type kimiAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &kimiAdapter{
		invoker:   NewInvoker(binaryPath, extraEnv),
		collector: NewCollector(),
	}
}

func (a *kimiAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *kimiAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

func (a *kimiAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
