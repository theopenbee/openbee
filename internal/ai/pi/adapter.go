package pi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type piAdapter struct {
	invoker *Invoker
}

// NewAdapter creates a piAdapter. extraEnv is injected into the subprocess environment.
func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &piAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *piAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	return setupWorkspace(workDir, role, opts)
}

func (a *piAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *piAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*piAdapter)(nil)
