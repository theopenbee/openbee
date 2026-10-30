package model

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func EnabledEngines(cfg api.Config) []api.EngineInfo {
	if cfg.Engines != nil {
		return slices.Clone(cfg.Engines)
	}
	engines := make([]api.EngineInfo, len(cfg.EnabledEngines))
	for i, id := range cfg.EnabledEngines {
		engines[i] = api.EngineInfo{ID: id}
	}
	return engines
}

func EngineIDs(engines []api.EngineInfo) []string {
	ids := make([]string, len(engines))
	for i, engine := range engines {
		ids[i] = engine.ID
	}
	return ids
}

func FindEngine(engines []api.EngineInfo, id string) api.EngineInfo {
	for _, engine := range engines {
		if engine.ID == id {
			return engine
		}
	}
	return api.EngineInfo{ID: id}
}

func DefaultEngine(systemDefault string, enabled []string) string {
	if slices.Contains(enabled, systemDefault) {
		return systemDefault
	}
	if len(enabled) > 0 {
		return enabled[0]
	}
	return ""
}

func AvailableEngine(chosen string, enabled []string) string {
	if slices.Contains(enabled, chosen) || len(enabled) == 0 {
		return chosen
	}
	return enabled[0]
}

func FollowedEngine(selected, systemDefault string, enabled []string) string {
	if selected != "" {
		return selected
	}
	if slices.Contains(enabled, systemDefault) {
		return systemDefault
	}
	return ""
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
