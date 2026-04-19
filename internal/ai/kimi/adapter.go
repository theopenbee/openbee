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
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type kimiAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &kimiAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *kimiAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *kimiAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	if err != nil {
		return ai.RunResult{}, err
	}
	return ai.RunResult{Process: proc, Output: out, ExtractResult: ExtractResultFromLog}, nil
}

var _ ai.EngineAdapter = (*kimiAdapter)(nil)
