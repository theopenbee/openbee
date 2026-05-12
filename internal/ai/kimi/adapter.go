package kimi

import (
	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineKimi), cfg.ExtraEnv()), nil
	})
}

// NewAdapter constructs a Kimi engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &ai.BaseAdapter{
		Invoker:   NewInvoker(binaryPath, extraEnv),
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	}
}
