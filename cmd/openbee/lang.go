package main

import (
	"github.com/theopenbee/openbee/internal/infra/config"
)

// detectLang determines the UI language using priority:
// config.yaml language field > default "en".
func detectLang() string {
	// Best-effort: try the default config path.
	if lang := config.GetLang("config.yaml"); lang != "" {
		return lang
	}
	return "en"
}
