package kimi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineKimi
		}
		return NewAdapter(path), nil
	})
}

type kimiAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath string) ai.EngineAdapter {
	return &kimiAdapter{invoker: NewInvoker(binaryPath)}
}

func (a *kimiAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *kimiAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *kimiAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*kimiAdapter)(nil)
