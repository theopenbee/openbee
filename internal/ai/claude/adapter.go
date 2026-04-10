package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

func (a *claudeAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	switch role {
	case ai.RoleWorker:
		if err := writeCLAUDEMD(workDir, ImportLine+"\n"); err != nil {
			return err
		}
		return EnsureSystemRules(workDir, ai.RoleWorker,
			WithName(opts.Name),
			WithDescription(opts.Description),
			WithMemory(opts.Memory),
		)
	case ai.RoleBee:
		if err := writeCLAUDEMD(workDir, ai.BeePersona+"\n"+ImportLine+"\n"); err != nil {
			return err
		}
		// Write .openbee.md directly; do not modify an existing CLAUDE.md.
		rulesPath := filepath.Join(workDir, SystemRulesFile)
		return os.WriteFile(rulesPath, []byte(ai.BeeRules()), 0o644)
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *claudeAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

func writeCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	return ai.CreateFileOnce(filepath.Join(workDir, "CLAUDE.md"), persona)
}
