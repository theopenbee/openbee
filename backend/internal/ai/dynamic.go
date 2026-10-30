package ai

import (
	"context"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

type DynamicAdapter struct {
	src EngineSource
	cfg *enginecfg.Store
}

func NewDynamicAdapter(src EngineSource, cfg *enginecfg.Store) *DynamicAdapter {
	return &DynamicAdapter{src: src, cfg: cfg}
}

func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error) {
	name := d.cfg.Get()
	if name == "" {
		return RunResult{}, ErrNoEngine
	}
	e, ok := d.src.Engine(name)
	if !ok {
		return RunResult{}, ErrEngineUnavailable(name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

func (d *DynamicAdapter) CollectTokenUsage(context.Context, []string) (map[string][]TokenUsage, error) {
	return nil, nil
}
