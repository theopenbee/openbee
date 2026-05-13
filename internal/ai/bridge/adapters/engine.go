package adapters

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

type engineSelector struct {
	engines map[string]ai.EngineAdapter
	cfg     *enginecfg.Store
}

func NewEngineSelector(engines map[string]ai.EngineAdapter, cfg *enginecfg.Store) bridge.EngineSelector {
	return engineSelector{engines: engines, cfg: cfg}
}

func (s engineSelector) ForWorker(hint string) string {
	if hint != "" {
		if _, ok := s.engines[hint]; ok {
			return hint
		}
	}
	return s.cfg.Get()
}
func (s engineSelector) ForBee() string { return s.cfg.Get() }
