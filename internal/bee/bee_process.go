package bee

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/theopenbee/openbee/internal/auth"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/config"
)

// DefaultPersona is the hardcoded bee persona content for CLAUDE.md.
const DefaultPersona = `You are B, an AI assistant.`

// BeeProcess represents a single short-lived bee Claude invocation.
type BeeProcess struct {
	invoker     *claude.Invoker
	tokenSecret string
	tokenTTL    time.Duration
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig) *BeeProcess {
	return &BeeProcess{
		invoker:     claude.NewInvoker(cfg.Claude.Path, cfg.MCPBaseURL),
		tokenSecret: cfg.MCP.TokenSecret,
		tokenTTL:    cfg.MCP.TokenTTL,
	}
}

// WriteCLAUDEMD creates the CLAUDE.md file in workDir with persona content
// only if it does not already exist. This preserves any user edits.
func WriteCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists, do not overwrite
	}
	return os.WriteFile(path, []byte(persona), 0o644)
}

// Run spawns the bee process, redirecting output to logPath.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error) {
	token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("generate bee token: %w", err)
	}
	opts.APIKey = token
	return p.invoker.Run(ctx, workDir, prompt, opts, logPath)
}
