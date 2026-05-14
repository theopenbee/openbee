package bridge

import (
	"fmt"
	"os"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type EngineSet struct {
	adapters map[string]ai.EngineAdapter
}

func BuildEngines(rpcBaseURL string, isEnabled func(string) bool, rawFor func(string) map[string]any) (EngineSet, error) {
	os.Setenv("OPENBEE_URL", rpcBaseURL) //nolint:errcheck
	result := make(map[string]ai.EngineAdapter)
	for _, name := range AllEngines() {
		if !isEnabled(name) {
			continue
		}
		adapter, err := ai.New(name, ai.EngineConfig{Raw: rawFor(name)})
		if err != nil {
			return EngineSet{}, fmt.Errorf("init engine %q: %w", name, err)
		}
		result[name] = adapter
	}
	return EngineSet{adapters: result}, nil
}

func EngineSetForTest(adapters map[string]ai.EngineAdapter) EngineSet {
	return EngineSet{adapters: adapters}
}
