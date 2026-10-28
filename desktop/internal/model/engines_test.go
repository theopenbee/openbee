package model

import (
	"maps"
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
		{"uses the built-in default when no engine is enabled", "pi", nil, "claude"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultEngine(tc.systemDefault, tc.enabled); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnabledEngines(t *testing.T) {
	tests := []struct {
		name string
		cfg  api.Config
		want []string
	}{
		{"all engines when the config lists none", api.Config{}, []string{"claude", "codex", "pi"}},
		{"all engines for an empty list", api.Config{EnabledEngines: []string{}}, []string{"claude", "codex", "pi"}},
		{"keeps the canonical order", api.Config{EnabledEngines: []string{"pi", "claude"}}, []string{"claude", "pi"}},
		{"drops unknown engines", api.Config{EnabledEngines: []string{"gemini", "codex"}}, []string{"codex"}},
		{"ignores duplicates", api.Config{EnabledEngines: []string{"pi", "pi"}}, []string{"pi"}},
		{"nothing known leaves nothing", api.Config{EnabledEngines: []string{"gemini"}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EnabledEngines(tc.cfg); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnabledEnginesReturnsACopy(t *testing.T) {
	got := EnabledEngines(api.Config{})
	got[0] = "changed"
	if AllEngines[0] != "claude" {
		t.Fatalf("AllEngines was mutated: %v", AllEngines)
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
