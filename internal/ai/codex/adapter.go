package codex

import (
	"context"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.OpenbeeURL, cfg.ExtraEnv())
	})
}

type codexAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL, store, extraEnv)}, nil
}

func (a *codexAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

var _ ai.EngineAdapter = (*codexAdapter)(nil)
