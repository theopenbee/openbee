package main

import (
	"os"
	"strings"

	"github.com/theopenbee/openbee/internal/config"
)

// parseLangFlag scans os.Args for --lang <value> or --lang=<value>
// without triggering cobra parsing. Returns empty string if not found.
func parseLangFlag(args []string) string {
	for i, arg := range args {
		if arg == "--lang" && i+1 < len(args) {
			return args[i+1]
		}
		if val, ok := strings.CutPrefix(arg, "--lang="); ok {
			return val
		}
	}
	return ""
}

// detectLang determines the UI language using priority:
// CLI flag > OPENBEE_LANG env > config.yaml language field > default "zh".
// flagLang is the value extracted by parseLangFlag (may be empty).
func detectLang(flagLang string) string {
	if flagLang != "" {
		return flagLang
	}
	if env := os.Getenv("OPENBEE_LANG"); env != "" {
		return env
	}
	// Best-effort: try the default config path.
	if lang := config.GetLang("config.yaml"); lang != "" {
		return lang
	}
	return "zh"
}
