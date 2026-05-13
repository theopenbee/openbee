package codex

import (
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
	ai.RegisterEngine(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.ExtraEnv())
	})
}

// NewAdapter constructs a Codex engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return core.NewEngineAdapter(NewBackend(binaryPath, store, extraEnv)), nil
}
