package pi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("pi", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "pi"
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type piAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &piAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *piAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *piAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *piAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*piAdapter)(nil)
