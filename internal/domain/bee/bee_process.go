package bee

import (
	"context"
	"encoding/json"
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
	opts.ExtraArgs = p.resolveExtraArgs(ctx)
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}

func (p *BeeProcess) resolveExtraArgs(ctx context.Context) []string {
	engineName := p.engineCfg.Get()
	globalMap := p.loadExtraArgs(ctx, model.SystemConfigKeyEngineExtraArgsGlobal)
	beeMap := p.loadExtraArgs(ctx, model.SystemConfigKeyEngineExtraArgsBee)
	merged := ai.MergeEngineExtraArgs(globalMap, beeMap)
	return merged[engineName]
}

func (p *BeeProcess) loadExtraArgs(ctx context.Context, key string) ai.EngineExtraArgsMap {
	if p.sysConfigStore == nil {
		return nil
	}
	cfg, found, err := p.sysConfigStore.Get(ctx, key)
	if err != nil || !found || cfg.Value == "" || cfg.Value == "{}" {
		return nil
	}
	var raw map[string]string
	if json.Unmarshal([]byte(cfg.Value), &raw) != nil {
		return nil
	}
	parsed, _ := ai.ParseEngineExtraArgs(raw)
	return parsed
}
