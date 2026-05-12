package pi

import (
	ai "github.com/theopenbee/openbee/internal/ai"
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
	return &ai.BaseAdapter{
		Invoker:   inv,
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	}, nil
}
