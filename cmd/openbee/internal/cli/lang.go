package cli

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// DetectLang determines the UI language using priority:
// config.yaml language field > default English.
func DetectLang() string {
	if lang := config.GetLang("config.yaml"); lang != "" {
		return lang
	}
	return i18n.LangEN
}
