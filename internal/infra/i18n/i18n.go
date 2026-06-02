package i18n

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

// Language codes used throughout the CLI.
const (
	LangEN = "en"
	LangZH = "zh"
)

// M is the global translation instance. Initialized (once) by Load() at the
// earliest point in main(); read-only after that.
var M = &Messages{}

// SupportedLangs lists all supported language codes.
var SupportedLangs = []string{LangZH, LangEN}

// Load loads the translation file for the given language and sets M.
// If the language is not supported (file not found), it silently falls back to zh.
func Load(lang string) error {
	data, err := localesFS.ReadFile("locales/" + lang + ".yaml")
	if err != nil {
		data, err = localesFS.ReadFile("locales/zh.yaml")
		if err != nil {
			return fmt.Errorf("i18n: failed to load default locale: %w", err)
		}
	}
	m := &Messages{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return fmt.Errorf("i18n: parse locale %q: %w", lang, err)
	}
	M = m
	return nil
}
