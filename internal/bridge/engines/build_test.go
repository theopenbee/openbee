package engines_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/bridge/engines"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestBuildEnginesRegistersAdapters(t *testing.T) {
	set, err := engines.BuildEngines(
		"http://127.0.0.1:8080",
		func(name string) bool { return name == bridge.EngineClaude },
		func(string) map[string]any { return nil },
	)
	if err != nil {
		t.Fatalf("BuildEngines: %v", err)
	}

	svc := bridge.NewService(bridge.ServiceOptions{
		Engines:   set,
		EngineCfg: enginecfg.NewStore(bridge.EngineClaude),
	})
	if err := svc.ValidateEngine(bridge.EngineClaude); err != nil {
		t.Fatalf("ValidateEngine(%q): %v", bridge.EngineClaude, err)
	}
	got := svc.EnabledEngines()
	if len(got) != 1 || got[0] != bridge.EngineClaude {
		t.Fatalf("EnabledEngines() = %v, want [%s]", got, bridge.EngineClaude)
	}
}
