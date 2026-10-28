package l10n

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/api"
)

func ErrorText(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	if apiErr.Code != "" {
		if text, ok := errorText(apiErr.Code, apiErr.Params); ok {
			return text
		}
	}
	return apiErr.Message
}

func Localized(t api.LocalizedText, lang string) string {
	if len(t) == 0 {
		return ""
	}
	if text, ok := t[lang]; ok {
		return text
	}
	base := baseLanguage(lang)
	if text, ok := t[base]; ok {
		return text
	}
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if baseLanguage(key) == base {
			return t[key]
		}
	}
	if text, ok := t["en"]; ok {
		return text
	}
	return t[keys[0]]
}

func Lang() string {
	return langOf(mygo.App.Locale())
}

func SetLang(lang string) {
	mygo.App.SetLocale(localeOf(lang))
}

func langOf(locale string) string {
	if baseLanguage(locale) == "zh" {
		return "zh"
	}
	return "en"
}

func localeOf(lang string) string {
	if baseLanguage(lang) == "zh" {
		return "zh-Hans"
	}
	return "en"
}

func baseLanguage(tag string) string {
	base, _, _ := strings.Cut(strings.ReplaceAll(tag, "_", "-"), "-")
	return strings.ToLower(base)
}

func param(params map[string]any, name string) string {
	value, ok := params[name]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
