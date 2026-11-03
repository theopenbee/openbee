package i18n

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

// M is the global translation instance. Initialized by Load() when the server
// starts; read-only after that.
var M = &Messages{}

// Load loads the translation file for the given language and sets M.
// If the language is not supported (file not found), it silently falls back to en.
func Load(lang string) error {
	data, err := localesFS.ReadFile("locales/" + lang + ".yaml")
	if err != nil {
		data, err = localesFS.ReadFile("locales/en.yaml")
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
