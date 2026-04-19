package claude

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineClaude
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type claudeAdapter struct {
	invoker *Invoker
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", ai.SystemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	if err != nil {
		return ai.RunResult{}, err
	}
	return ai.RunResult{Process: proc, Output: out, ExtractResult: ExtractResultFromLog}, nil
}
