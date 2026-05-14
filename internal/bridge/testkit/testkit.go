package testkit

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// Adapter is a no-op EngineAdapter for tests that need a bridge.Service.
type Adapter struct{}

func (Adapter) Prepare(string, ai.PrepareOptions) error { return nil }

func (Adapter) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{ExtractResult: func(string) string { return "" }}, nil
}

func (Adapter) CollectTokenUsage(context.Context, string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func NewBridge() bridge.Bridge {
	return bridge.NewService(bridge.ServiceOptions{
		Engines:     bridge.EngineSetForTest(map[string]ai.EngineAdapter{bridge.EngineClaude: Adapter{}}),
		EngineCfg:   enginecfg.NewStore(bridge.EngineClaude),
		TokenSecret: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
	})
}
