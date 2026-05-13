package adapters

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// sysConfigReader is the subset of *store.SystemConfigStore used here.
type sysConfigReader interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}

type argsResolver struct{ store sysConfigReader }

func NewArgsResolver(store sysConfigReader) bridge.ArgsResolver { return argsResolver{store: store} }

func (a argsResolver) read(ctx context.Context, key string) string {
	if a.store == nil {
		return ""
	}
	cfg, ok, err := a.store.Get(ctx, key)
	if err != nil || !ok {
		return ""
	}
	return cfg.Value
}

func (a argsResolver) ForWorker(ctx context.Context, workerEngineArgs, engineName string) string {
	return ai.ResolveExtraArgs(engineName, a.read(ctx, model.SystemConfigKeyEngineArgsGlobal), workerEngineArgs)
}
func (a argsResolver) ForBee(ctx context.Context, engineName string) string {
	return ai.ResolveExtraArgs(engineName,
		a.read(ctx, model.SystemConfigKeyEngineArgsGlobal),
		a.read(ctx, model.SystemConfigKeyEngineArgsBee),
	)
}
