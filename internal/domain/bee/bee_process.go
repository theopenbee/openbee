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
		tokenSecret:    cfg.MCP.TokenSecret,
		tokenTTL:       cfg.MCP.TokenTTL,
		envService:     envSvc,
		sysConfigStore: sysStore,
		engineCfg:      engineCfg,
	}
}

func (p *BeeProcess) Prepare(workDir string, opts ai.PrepareOptions) error {
	return p.engine.Prepare(workDir, opts)
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
	opts.ExtraEnv = extraEnv
	opts.APIKey = token
	opts.ExtraArgs = p.resolveEngineArgs(ctx)
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}

func (b *BeeProcess) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.engine.CollectTokenUsage(ctx, sessionID)
}

func (p *BeeProcess) resolveEngineArgs(ctx context.Context) []string {
	engineName := p.engineCfg.Get()
	globalMap := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
	beeMap := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsBee)
	merged := ai.MergeEngineArgs(globalMap, beeMap)
	return merged[engineName]
}

func (p *BeeProcess) loadEngineArgs(ctx context.Context, key string) ai.EngineArgsMap {
	if p.sysConfigStore == nil {
		return nil
	}
	cfg, found, err := p.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return nil
	}
	return ai.ParseEngineArgsJSON(cfg.Value)
}
