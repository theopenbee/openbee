package model

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

var AllEngines = []string{"claude", "codex", "pi"}

const fallbackEngine = "claude"

func EnabledEngines(cfg api.Config) []string {
	if len(cfg.EnabledEngines) == 0 {
		return slices.Clone(AllEngines)
	}
	var out []string
	for _, engine := range AllEngines {
		if slices.Contains(cfg.EnabledEngines, engine) {
			out = append(out, engine)
		}
	}
	return out
}

func DefaultEngine(systemDefault string, enabled []string) string {
	if slices.Contains(enabled, systemDefault) {
		return systemDefault
	}
	if len(enabled) > 0 {
		return enabled[0]
	}
	return fallbackEngine
}

func AvailableEngine(chosen string, enabled []string) string {
	if slices.Contains(enabled, chosen) || len(enabled) == 0 {
		return chosen
	}
	return enabled[0]
}

func CleanEngineArgs(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for engine, args := range m {
		if strings.TrimSpace(args) != "" {
			out[engine] = args
		}
	}
	return out
}

func ParseEngineArgsConfig(raw string) map[string]string {
	out := map[string]string{}
	var parsed map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &parsed) != nil {
		return out
	}
	for engine, value := range parsed {
		if args, ok := value.(string); ok {
			out[engine] = args
		}
	}
	return out
}
