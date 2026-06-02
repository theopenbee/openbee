package cli

import (
	"github.com/theopenbee/openbee/internal/infra/config"
)

// DetectLang determines the UI language using priority:
// config.yaml language field > default "en".
func DetectLang() string {
	if lang := config.GetLang("config.yaml"); lang != "" {
		return lang
	}
	return "en"
}
