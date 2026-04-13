package codex

import (
	"context"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineCodex
		}
		return NewAdapter(path, cfg.OpenbeeURL)
	})
}

type codexAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL, store)}, nil
}

func (a *codexAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *codexAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*codexAdapter)(nil)
