package codex

import (
	"context"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.ExtraEnv())
	})
}

type codexAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{
		invoker:   NewInvoker(binaryPath, store, extraEnv),
		collector: NewCollector(),
	}, nil
}

func (a *codexAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

func (a *codexAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}

var _ ai.EngineAdapter = (*codexAdapter)(nil)
