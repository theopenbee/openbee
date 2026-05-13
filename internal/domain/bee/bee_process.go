package bee

import (
	"context"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// defaultBeeID is the scope_id used for bee-scoped env configs. Since there is a
// single bee instance, all bee-scoped configs are stored under this well-known ID.
const defaultBeeID = "default"

type sysConfigReader interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}

// BeeProcess wraps an EngineAdapter and injects a short-lived auth token into each Run call.
type BeeProcess struct {
	engine         ai.EngineAdapter
	tokenSecret    string
	tokenTTL       time.Duration
	envService     *env.Service
	sysConfigStore sysConfigReader
	engineCfg      *enginecfg.Store
}

func NewBeeProcess(cfg config.BeeConfig, engine ai.EngineAdapter, envSvc *env.Service, sysStore sysConfigReader, engineCfg *enginecfg.Store) *BeeProcess {
	return &BeeProcess{
		engine:         engine,
		tokenSecret:    cfg.RPC.TokenSecret,
		tokenTTL:       cfg.RPC.TokenTTL,
		envService:     envSvc,
		sysConfigStore: sysStore,
		engineCfg:      engineCfg,
	}
}

// Run injects a bee auth token then delegates to the engine.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
	if err != nil {
		return ai.RunResult{}, fmt.Errorf("generate bee token: %w", err)
	}

	extraEnv, err := p.envService.ResolveBeeEnv(defaultBeeID)
	if err != nil {
		return ai.RunResult{}, fmt.Errorf("resolve bee env: %w", err)
	}

	globalJSON := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsGlobal)
	beeJSON := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsBee)

	opts.ExtraEnv = extraEnv
	opts.APIKey = token
	opts.ExtraArgs = ai.ResolveExtraArgs(p.engineCfg.Get(), globalJSON, beeJSON)
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}

func (b *BeeProcess) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.engine.CollectTokenUsage(ctx, sessionID)
}

// readSysConfig returns the raw config value, or "" on miss / read error.
// Errors are deliberately swallowed: a missing or corrupt engine_args row
// must not block the bee from running.
func (p *BeeProcess) readSysConfig(ctx context.Context, key string) string {
	if p.sysConfigStore == nil {
		return ""
	}
	cfg, found, err := p.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return ""
	}
	return cfg.Value
}
