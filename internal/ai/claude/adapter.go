package claude

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("claude", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "claude"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type claudeAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

func (a *claudeAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *claudeAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}
