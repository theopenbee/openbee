package kit_test

import (
	"reflect"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestEngineOptions(t *testing.T) {
	engines := []api.EngineInfo{
		{ID: "pi", Name: api.LocalizedText{"en": "Pi"}},
		{ID: "gemini"},
	}
	tests := []struct {
		name   string
		stored string
		want   []kit.SelectOption
	}{
		{"lists the engines by name or id", "", []kit.SelectOption{{Value: "pi", Text: "Pi"}, {Value: "gemini", Text: "gemini"}}},
		{"keeps an installed stored engine once", "pi", []kit.SelectOption{{Value: "pi", Text: "Pi"}, {Value: "gemini", Text: "gemini"}}},
		{
			"appends a stored engine that is not installed as disabled",
			"gone",
			[]kit.SelectOption{
				{Value: "pi", Text: "Pi"},
				{Value: "gemini", Text: "gemini"},
				{Value: "gone", Text: locales.WorkersFormEngineUnavailable("gone"), Disabled: true},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := kit.EngineOptions(engines, tc.stored); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestEngineOptionsWithoutEngines(t *testing.T) {
	if got := kit.EngineOptions(nil, ""); len(got) != 0 {
		t.Fatalf("got %#v, want no options", got)
	}
	got := kit.EngineOptions(nil, "gone")
	if len(got) != 1 || !got[0].Disabled || got[0].Value != "gone" {
		t.Fatalf("got %#v, want only the disabled stored engine", got)
	}
}
