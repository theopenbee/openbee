package pi

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
	ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.ExtraEnv())
	})
}

// NewAdapter constructs a pi engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, extraEnv)
	if err != nil {
		return nil, err
	}
	return &core.Composite{
		Invoker:   inv,
		Collector: NewCollector(),
		Extractor: Extractor{},
	}, nil
}
