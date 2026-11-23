package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxModelLength = 200

func ValidateModel(value string) error {
	if utf8.RuneCountInString(value) > MaxModelLength {
		return fmt.Errorf("model is longer than %d characters", MaxModelLength)
	}
	if strings.HasPrefix(value, "-") {
		return errors.New(`model must not start with "-"`)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return errors.New("model must not contain whitespace or control characters")
		}
	}
	return nil
}

func CleanEngineModels(raw map[string]string) map[string]string {
	out := make(map[string]string, len(raw))
	for engine, name := range raw {
		if name = strings.TrimSpace(name); engine != "" && name != "" {
			out[engine] = name
		}
	}
	return out
}

func ParseEngineModelsJSON(value string) map[string]string {
	var raw map[string]string
	if value == "" || json.Unmarshal([]byte(value), &raw) != nil {
		return nil
	}
	return CleanEngineModels(raw)
}

func ResolveEngineModel(engine string, layers ...map[string]string) string {
	for _, models := range layers {
		if name := models[engine]; name != "" {
			return name
		}
	}
	return ""
}

func TakeModelArg(raw string) (name, rest string, ok bool) {
	tokens, err := splitCLIArgs(raw)
	if err != nil {
		return "", "", false
	}
	kept := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		switch token := tokens[i]; {
		case token == "--model" && i+1 < len(tokens):
			name, ok = tokens[i+1], true
			i++
		case strings.HasPrefix(token, "--model="):
			name, ok = strings.TrimPrefix(token, "--model="), true
		default:
			kept = append(kept, token)
		}
	}
	if !ok || name == "" {
		return "", "", false
	}
	return name, JoinCLIArgs(kept), true
}

func MoveModelArgs(args, models map[string]string) bool {
	moved := false
	for engine, raw := range args {
		if strings.TrimSpace(models[engine]) != "" {
			continue
		}
		name, rest, ok := TakeModelArg(raw)
		if !ok || ValidateModel(name) != nil {
			continue
		}
		models[engine] = name
		if rest == "" {
			delete(args, engine)
		} else {
			args[engine] = rest
		}
		moved = true
	}
	return moved
}

func JoinCLIArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteCLIArg(arg)
	}
	return strings.Join(quoted, " ")
}

func quoteCLIArg(arg string) string {
	plain := arg != "" && !strings.ContainsFunc(arg, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`'"\`, r)
	})
	if plain {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
