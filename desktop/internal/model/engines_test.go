package model

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestAvailableEngine(t *testing.T) {
	tests := []struct {
		name    string
		chosen  string
		enabled []string
		want    string
	}{
		{"keeps an enabled engine", "pi", []string{"claude", "pi"}, "pi"},
		{"falls back to the first enabled engine when the seeded one is disabled", "claude", []string{"pi"}, "pi"},
		{"keeps the engine when the enabled list is empty", "codex", nil, "codex"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AvailableEngine(tc.chosen, tc.enabled); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultEngine(t *testing.T) {
	tests := []struct {
		name          string
		systemDefault string
		enabled       []string
		want          string
	}{
		{"preselects the system default engine when it is enabled", "pi", []string{"claude", "codex", "pi"}, "pi"},
		{"falls back to the first enabled engine when the default is disabled", "claude", []string{"codex", "pi"}, "codex"},
		{"falls back to the first enabled engine when the default is unset", "", []string{"pi"}, "pi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultEngine(tc.systemDefault, tc.enabled); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultEngine_NoneEnabled(t *testing.T) {
	for _, enabled := range [][]string{nil, {}} {
		if got := DefaultEngine("pi", enabled); got != "" {
			t.Fatalf("got %q for %v, want an empty engine", got, enabled)
		}
	}
}

func TestEnabledEngines_Empty(t *testing.T) {
	for _, cfg := range []api.Config{
		{},
		{Engines: []api.EngineInfo{}},
		{Engines: []api.EngineInfo{}, EnabledEngines: []string{}},
	} {
		if got := EnabledEngines(cfg); len(got) != 0 {
			t.Fatalf("EnabledEngines(%+v) = %v, want none", cfg, got)
		}
	}
}

func TestEnabledEngines_OldServerIDs(t *testing.T) {
	got := EnabledEngines(api.Config{EnabledEngines: []string{"pi", "claude"}})
	want := []api.EngineInfo{{ID: "pi"}, {ID: "claude"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestEnabledEngines_FromEngines(t *testing.T) {
	cfg := api.Config{
		EnabledEngines: []string{"claude"},
		Engines: []api.EngineInfo{
			{ID: "pi", Name: api.LocalizedText{"en": "Pi", "zh": "Pi"}},
			{ID: "gemini", Name: api.LocalizedText{"en": "Gemini CLI"}},
		},
	}
	got := EnabledEngines(cfg)
	if !reflect.DeepEqual(got, cfg.Engines) {
		t.Fatalf("got %#v, want the engines of the server in its order", got)
	}
}

func TestEnabledEnginesReturnsACopy(t *testing.T) {
	cfg := api.Config{Engines: []api.EngineInfo{{ID: "pi"}}}
	got := EnabledEngines(cfg)
	got[0].ID = "changed"
	if cfg.Engines[0].ID != "pi" {
		t.Fatalf("the config engines were mutated: %v", cfg.Engines)
	}
}

func TestEngineIDs(t *testing.T) {
	got := EngineIDs([]api.EngineInfo{{ID: "pi"}, {ID: "claude"}})
	if !slices.Equal(got, []string{"pi", "claude"}) {
		t.Fatalf("got %v", got)
	}
	if got := EngineIDs(nil); len(got) != 0 {
		t.Fatalf("got %v for no engines", got)
	}
}

func TestFindEngine(t *testing.T) {
	engines := []api.EngineInfo{{ID: "pi", Name: api.LocalizedText{"en": "Pi"}}}
	if got := FindEngine(engines, "pi"); got.ID != "pi" || got.Name["en"] != "Pi" {
		t.Fatalf("got %#v", got)
	}
	if got := FindEngine(engines, "gone"); !reflect.DeepEqual(got, api.EngineInfo{ID: "gone"}) {
		t.Fatalf("got %#v for an unknown engine, want one carrying only the id", got)
	}
}

func TestFollowedEngine(t *testing.T) {
	enabled := []string{"claude", "pi"}
	tests := []struct {
		name          string
		selected      string
		systemDefault string
		want          string
	}{
		{"keeps the selected engine", "pi", "claude", "pi"},
		{"keeps a selected engine that is not installed", "gone", "claude", "gone"},
		{"follows an installed system default", "", "claude", "claude"},
		{"follows nothing for an uninstalled system default", "", "gone", ""},
		{"follows nothing without a system default", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FollowedEngine(tc.selected, tc.systemDefault, enabled); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanEngineArgs(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{"drops empty and blank values", map[string]string{"claude": "", "codex": "  \t", "pi": "--x"}, map[string]string{"pi": "--x"}},
		{"keeps values untouched", map[string]string{"claude": " --effort high "}, map[string]string{"claude": " --effort high "}},
		{"nil becomes empty", nil, map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanEngineArgs(tc.in)
			if got == nil || !maps.Equal(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseEngineArgsConfig(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{"empty string", "", map[string]string{}},
		{"object", `{"claude":"--effort high","pi":"--x"}`, map[string]string{"claude": "--effort high", "pi": "--x"}},
		{"empty object", `{}`, map[string]string{}},
		{"invalid json", `{"claude":`, map[string]string{}},
		{"array", `["a"]`, map[string]string{}},
		{"null", `null`, map[string]string{}},
		{"string", `"x"`, map[string]string{}},
		{"keeps only string values", `{"claude":"--x","pi":3,"codex":null}`, map[string]string{"claude": "--x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseEngineArgsConfig(tc.raw)
			if got == nil || !maps.Equal(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
