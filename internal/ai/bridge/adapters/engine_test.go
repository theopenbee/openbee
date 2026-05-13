package adapters

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestEngineSelectorForWorkerPrefersHintFallsBackToDefault(t *testing.T) {
	cfg := enginecfg.NewStore(ai.EngineClaude)
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: nil, ai.EngineCodex: nil}
	s := NewEngineSelector(engines, cfg)

	if got := s.ForWorker(""); got != ai.EngineClaude {
		t.Fatalf("empty hint: got %q", got)
	}
	if got := s.ForWorker(ai.EngineCodex); got != ai.EngineCodex {
		t.Fatalf("valid hint: got %q", got)
	}
	if got := s.ForWorker("missing"); got != ai.EngineClaude {
		t.Fatalf("unknown hint should fall back: got %q", got)
	}
}

func TestEngineSelectorForBee(t *testing.T) {
	cfg := enginecfg.NewStore(ai.EngineKimi)
	s := NewEngineSelector(map[string]ai.EngineAdapter{ai.EngineKimi: nil}, cfg)
	if got := s.ForBee(); got != ai.EngineKimi {
		t.Fatalf("got %q", got)
	}
}
