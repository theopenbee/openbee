package pi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.OpenbeeURL, cfg.ExtraEnv())
	})
}

type piAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, openbeeURL, extraEnv)
	if err != nil {
		return nil, err
	}
	return &piAdapter{invoker: inv}, nil
}

func (a *piAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *piAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

var _ ai.EngineAdapter = (*piAdapter)(nil)
