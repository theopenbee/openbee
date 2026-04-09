package codex

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("codex", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "codex"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type codexAdapter struct {
	invoker *Invoker
}

// NewAdapter creates a codexAdapter.
func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

func (a *codexAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	return setupWorkspace(workDir, role, opts)
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *codexAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

// Compile-time interface checks.
var _ ai.EngineAdapter = (*codexAdapter)(nil)
var _ ai.Process = (*Process)(nil)
