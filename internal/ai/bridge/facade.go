package bridge

import (
	"context"
	"errors"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Bridge is the single business-facing entry point.
type Bridge interface {
	RunWorker(ctx context.Context, req WorkerRunRequest) (Handle, error)
	RunBee(ctx context.Context, req BeeRunRequest) (Handle, error)

	AllEngines() []string
	EnabledEngines() []string
	IsEnabled(name string) bool
	ValidateEngine(name string) error
	ValidateEngineArgs(line string) error

	ResolveEngineForWorker(workerID, hint string) string
	ResolveEngineForBee() string

	CollectUsage(ctx context.Context, engineName, sessionID string) ([]Usage, error)
}

// Deps groups the five dependency ports the bridge needs.
type Deps struct {
	TokenIssuer     TokenIssuer
	EnvResolver     EnvResolver
	EngineSelector  EngineSelector
	ArgsResolver    ArgsResolver
	LogPathProvider LogPathProvider
}

// Config is the constructor input.
type Config struct {
	Engines map[string]ai.EngineAdapter
	Deps    Deps
}

// ErrInvalidConfig is returned by New when Config is incomplete.
var ErrInvalidConfig = errors.New("bridge: invalid config")

// New returns a Bridge. It validates that all five ports are non-nil and
// that Engines is non-empty.
func New(cfg Config) (Bridge, error) {
	switch {
	case len(cfg.Engines) == 0:
		return nil, errors.New("bridge: Config.Engines is empty: " + ErrInvalidConfig.Error())
	case cfg.Deps.TokenIssuer == nil:
		return nil, errors.New("bridge: Deps.TokenIssuer is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.EnvResolver == nil:
		return nil, errors.New("bridge: Deps.EnvResolver is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.EngineSelector == nil:
		return nil, errors.New("bridge: Deps.EngineSelector is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.ArgsResolver == nil:
		return nil, errors.New("bridge: Deps.ArgsResolver is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.LogPathProvider == nil:
		return nil, errors.New("bridge: Deps.LogPathProvider is nil: " + ErrInvalidConfig.Error())
	}
	return &bridgeImpl{engines: cfg.Engines, deps: cfg.Deps}, nil
}

// bridgeImpl declared in deps-bearing form. Methods live in this file
// (engine resolution) plus names.go / validate.go / usage.go / run.go.
type bridgeImpl struct {
	engines map[string]ai.EngineAdapter
	deps    Deps
}

func (b *bridgeImpl) ResolveEngineForWorker(_, hint string) string {
	return b.deps.EngineSelector.ForWorker(hint)
}

func (b *bridgeImpl) ResolveEngineForBee() string {
	return b.deps.EngineSelector.ForBee()
}
