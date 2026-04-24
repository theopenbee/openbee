package ai

import "strings"

// EngineExtraArgsMap maps engine name -> (arg key -> arg value).
// Boolean flags use an empty string as value.
type EngineExtraArgsMap map[string]map[string]string

// ParseEngineExtraArgs parses raw CLI strings per engine into a structured map.
func ParseEngineExtraArgs(raw map[string]string) (EngineExtraArgsMap, error) {
	result := make(EngineExtraArgsMap, len(raw))
	for engine, s := range raw {
		result[engine] = parseArgString(s)
	}
	return result, nil
}

func parseArgString(s string) map[string]string {
	tokens := strings.Fields(s)
	m := make(map[string]string)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		key := strings.TrimPrefix(tok, "--")
		if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "--") {
			m[key] = tokens[i+1]
			i++
		} else {
			m[key] = ""
		}
	}
	return m
}

// MergeEngineExtraArgs merges two maps; override wins on conflicting keys.
func MergeEngineExtraArgs(base, override EngineExtraArgsMap) EngineExtraArgsMap {
	result := make(EngineExtraArgsMap, len(base))
	for engine, args := range base {
		cp := make(map[string]string, len(args))
		for k, v := range args {
			cp[k] = v
		}
		result[engine] = cp
	}
	for engine, overrideArgs := range override {
		if result[engine] == nil {
			result[engine] = make(map[string]string, len(overrideArgs))
		}
		for k, v := range overrideArgs {
			result[engine][k] = v
		}
	}
	return result
}

// BuildExtraArgSlice converts a single engine's arg map into a CLI arg slice.
func BuildExtraArgSlice(args map[string]string) []string {
	out := make([]string, 0, len(args)*2)
	for k, v := range args {
		out = append(out, "--"+k)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
