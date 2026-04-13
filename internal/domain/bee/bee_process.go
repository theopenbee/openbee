package bee

import (
	"context"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// BeeProcess wraps an EngineAdapter and injects a short-lived auth token into each Run call.
type BeeProcess struct {
	engine      ai.EngineAdapter
	tokenSecret string
	tokenTTL    time.Duration
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig, engine ai.EngineAdapter) *BeeProcess {
	return &BeeProcess{
		engine:      engine,
		tokenSecret: cfg.MCP.TokenSecret,
		tokenTTL:    cfg.MCP.TokenTTL,
	}
}

func (p *BeeProcess) Prepare(workDir string, opts ai.PrepareOptions) error {
	return p.engine.Prepare(workDir, opts)
}

func (p *BeeProcess) ExtractResult(logPath string) string {
	return p.engine.ExtractResult(logPath)
}

// Run injects a bee auth token then delegates to the engine.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("generate bee token: %w", err)
	}
	opts.APIKey = token
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}
