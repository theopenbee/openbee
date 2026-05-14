package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

type EngineArgsMap map[string][]string

func ParseEngineArgs(raw map[string]string) (EngineArgsMap, error) {
	parsed, err := ai.ParseEngineArgs(raw)
	if err != nil {
		return nil, err
	}
	return EngineArgsMap(parsed), nil
}

func ParseEngineArgsJSON(value string) EngineArgsMap {
	return EngineArgsMap(ai.ParseEngineArgsJSON(value))
}

func MergeEngineArgs(base, override EngineArgsMap) EngineArgsMap {
	merged := ai.MergeEngineArgs(ai.EngineArgsMap(base), ai.EngineArgsMap(override))
	return EngineArgsMap(merged)
}
