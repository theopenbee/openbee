package l10n

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestErrorTextCode(t *testing.T) {
	err := &api.Error{Status: 400, Message: "raw backend text", Code: "plugin_unknown_field", Params: map[string]any{"key": "x"}}
	got := ErrorText(err)
	if got != "x is not a setting of this plugin" {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextCodeWithoutParams(t *testing.T) {
	err := &api.Error{Status: 404, Message: "raw", Code: "worker_not_found"}
	if got := ErrorText(err); got != "Worker not found" {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextMissingParamIsEmpty(t *testing.T) {
	err := &api.Error{Code: "env_invalid_scope"}
	got := ErrorText(err)
	if strings.Contains(got, "nil") || !strings.HasPrefix(got, "Invalid scope ") {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextNumericParam(t *testing.T) {
	err := &api.Error{Code: "env_invalid_scope", Params: map[string]any{"scope": float64(7)}}
	if got := ErrorText(err); !strings.Contains(got, "Invalid scope 7:") {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextInChinese(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh")
	err := &api.Error{Code: "plugin_unknown_field", Params: map[string]any{"key": "x"}}
	got := ErrorText(err)
	if !strings.Contains(got, "x") || strings.Contains(got, "is not a setting") {
		t.Errorf("ErrorText = %q, want the Chinese text", got)
	}
}

func TestErrorTextFallsBackToMessage(t *testing.T) {
	err := &api.Error{Status: 400, Message: "worker name is required", Code: "worker_validation"}
	if got := ErrorText(err); got != "worker name is required" {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextWithoutCode(t *testing.T) {
	err := &api.Error{Status: 500, Message: "boom"}
	if got := ErrorText(err); got != "boom" {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextPlainError(t *testing.T) {
	if got := ErrorText(errors.New("dial tcp: refused")); got != "dial tcp: refused" {
		t.Errorf("ErrorText = %q", got)
	}
	if got := ErrorText(nil); got != "" {
		t.Errorf("ErrorText(nil) = %q", got)
	}
}

func TestErrorTextWrapped(t *testing.T) {
	err := fmt.Errorf("load worker: %w", &api.Error{Code: "worker_not_found", Message: "raw"})
	if got := ErrorText(err); got != "Worker not found" {
		t.Errorf("ErrorText = %q", got)
	}
}

func TestErrorTextCoversWebCodes(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("../../../web/src/locales/en.json"))
	if err != nil {
		t.Fatal(err)
	}
	var web struct {
		Errors map[string]string `json:"errors"`
	}
	if err := json.Unmarshal(data, &web); err != nil {
		t.Fatal(err)
	}
	if len(web.Errors) == 0 {
		t.Fatal("no errors.* keys in the web catalog")
	}
	for code := range web.Errors {
		if text, ok := errorText(code, nil); !ok || text == "" {
			t.Errorf("errorText(%q) = %q, %v", code, text, ok)
		}
	}
	if _, ok := errorText("no_such_code", nil); ok {
		t.Error("errorText accepted an unknown code")
	}
}

func TestLocalized(t *testing.T) {
	cases := []struct {
		name string
		text api.LocalizedText
		lang string
		want string
	}{
		{"base language matches a regional key", api.LocalizedText{"zh-CN": "甲", "en": "A"}, "zh", "甲"},
		{"exact key", api.LocalizedText{"zh": "乙", "zh-CN": "甲", "en": "A"}, "zh", "乙"},
		{"falls back to en", api.LocalizedText{"zh-CN": "甲", "en": "A"}, "fr", "A"},
		{"falls back to the first value", api.LocalizedText{"de": "B"}, "zh", "B"},
		{"first value is deterministic", api.LocalizedText{"fr": "F", "de": "D"}, "zh", "D"},
		{"regional language finds its base", api.LocalizedText{"zh": "乙", "en": "A"}, "zh-CN", "乙"},
		{"regional language finds a sibling", api.LocalizedText{"zh-TW": "丙", "en": "A"}, "zh-CN", "丙"},
		{"empty", api.LocalizedText{}, "zh", ""},
		{"nil", nil, "en", ""},
	}
	for _, c := range cases {
		if got := Localized(c.text, c.lang); got != c.want {
			t.Errorf("%s: Localized(%v, %q) = %q, want %q", c.name, c.text, c.lang, got, c.want)
		}
	}
}

func TestLangOf(t *testing.T) {
	for locale, want := range map[string]string{
		"zh":      "zh",
		"zh-Hans": "zh",
		"zh-CN":   "zh",
		"ZH-hant": "zh",
		"en":      "en",
		"en-US":   "en",
		"en-XA":   "en",
		"ja":      "en",
		"":        "en",
	} {
		if got := langOf(locale); got != want {
			t.Errorf("langOf(%q) = %q, want %q", locale, got, want)
		}
	}
}

func TestLocaleOf(t *testing.T) {
	for lang, want := range map[string]string{
		"zh":    "zh-Hans",
		"zh-CN": "zh-Hans",
		"en":    "en",
		"fr":    "en",
		"":      "en",
	} {
		if got := localeOf(lang); got != want {
			t.Errorf("localeOf(%q) = %q, want %q", lang, got, want)
		}
	}
}
