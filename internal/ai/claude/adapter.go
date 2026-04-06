package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// DefaultPersona is the default CLAUDE.md content for the bee workspace.
const DefaultPersona = `You are B, an AI assistant.`

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

// NewAdapter creates a claudeAdapter. Exported for testing.
func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

// SetupWorkspace implements ai.EngineAdapter.
func (a *claudeAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	switch role {
	case ai.RoleWorker:
		claudeMD := filepath.Join(workDir, "CLAUDE.md")
		if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				return fmt.Errorf("mkdir workdir: %w", err)
			}
			if err := os.WriteFile(claudeMD, []byte(ImportLine+"\n"), 0o644); err != nil {
				return fmt.Errorf("create CLAUDE.md: %w", err)
			}
		}
		return EnsureSystemRules(workDir, ai.RoleWorker,
			WithName(opts.Name),
			WithDescription(opts.Description),
			WithMemory(opts.Memory),
		)
	case ai.RoleBee:
		if err := writeCLAUDEMD(workDir, DefaultPersona+"\n"+ImportLine+"\n"); err != nil {
			return err
		}
		// Write .openbee.md directly; do not modify an existing CLAUDE.md.
		rulesPath := filepath.Join(workDir, SystemRulesFile)
		return os.WriteFile(rulesPath, []byte(beeRules()), 0o644)
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

// Run implements ai.EngineAdapter.
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

// writeCLAUDEMD creates workDir/CLAUDE.md with persona only if it does not exist.
func writeCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	return os.WriteFile(path, []byte(persona), 0o644)
}
