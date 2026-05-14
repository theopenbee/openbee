package engines

import (
	"fmt"
	"os"

	ai "github.com/theopenbee/openbee/internal/ai"
	_ "github.com/theopenbee/openbee/internal/ai/claude"
	_ "github.com/theopenbee/openbee/internal/ai/codex"
	_ "github.com/theopenbee/openbee/internal/ai/kimi"
	_ "github.com/theopenbee/openbee/internal/ai/pi"
	"github.com/theopenbee/openbee/internal/bridge"
)

func BuildEngines(rpcBaseURL string, isEnabled func(string) bool, rawFor func(string) map[string]any) (bridge.EngineSet, error) {
	os.Setenv("OPENBEE_URL", rpcBaseURL) //nolint:errcheck
	result := make(map[string]ai.EngineAdapter)
	for _, name := range bridge.AllEngines() {
		if !isEnabled(name) {
			continue
		}
		adapter, err := ai.New(name, ai.EngineConfig{Raw: rawFor(name)})
		if err != nil {
			return bridge.EngineSet{}, fmt.Errorf("init engine %q: %w", name, err)
		}
		result[name] = adapter
	}
	return bridge.NewEngineSet(result), nil
}
