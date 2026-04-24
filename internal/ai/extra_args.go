package ai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// EngineExtraArgsMap maps engine name -> ordered CLI args.
type EngineExtraArgsMap map[string][]string

// ParseEngineExtraArgs tokenizes raw CLI strings per engine while preserving
// order, duplicates, and quoted values.
func ParseEngineExtraArgs(raw map[string]string) (EngineExtraArgsMap, error) {
	result := make(EngineExtraArgsMap, len(raw))
	for engine, s := range raw {
		args, err := splitCLIArgs(s)
		if err != nil {
			return nil, fmt.Errorf("engine %q: %w", engine, err)
		}
		result[engine] = args
	}
	return result, nil
}

func splitCLIArgs(s string) ([]string, error) {
	var (
		args      []string
		buf       strings.Builder
		inSingle  bool
		inDouble  bool
		escaped   bool
		tokenOpen bool
	)

	flush := func() {
		if !tokenOpen {
			return
		}
		args = append(args, buf.String())
		buf.Reset()
		tokenOpen = false
	}

	for _, r := range s {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
			tokenOpen = true

		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				buf.WriteRune(r)
			}
			tokenOpen = true

		case inDouble:
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}

		default:
			switch {
			case unicode.IsSpace(r):
				flush()
			case r == '\'':
				inSingle = true
				tokenOpen = true
			case r == '"':
				inDouble = true
				tokenOpen = true
			case r == '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

// MergeEngineExtraArgs merges base and override by appending override args
// after base args, so later flags can override earlier ones while preserving
// the original CLI ordering.
func MergeEngineExtraArgs(base, override EngineExtraArgsMap) EngineExtraArgsMap {
	result := make(EngineExtraArgsMap, len(base)+len(override))
	for engine, args := range base {
		result[engine] = slices.Clone(args)
	}
	for engine, overrideArgs := range override {
		result[engine] = append(result[engine], overrideArgs...)
	}
	return result
}

// BuildExtraArgSlice returns a defensive copy of a single engine's arg slice.
func BuildExtraArgSlice(args []string) []string {
	return slices.Clone(args)
}

// ParseEngineExtraArgsJSON parses a JSON-encoded map[engine]rawCLIString value
// (as stored in the DB) into an EngineExtraArgsMap. Returns nil for empty/unset values.
func ParseEngineExtraArgsJSON(value string) EngineExtraArgsMap {
	if value == "" || value == "{}" {
		return nil
	}
	var raw map[string]string
	if json.Unmarshal([]byte(value), &raw) != nil {
		return nil
	}
	parsed, _ := ParseEngineExtraArgs(raw)
	return parsed
}
