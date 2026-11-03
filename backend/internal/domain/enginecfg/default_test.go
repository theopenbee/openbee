package enginecfg_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func enabled(ids ...string) func() []string {
	return func() []string { return ids }
}

func TestStore_EffectiveDefault(t *testing.T) {
	cases := []struct {
		name          string
		selected      string
		configDefault string
		enabled       []string
		want          string
	}{
		{name: "SelectedEnabled", selected: "codex", configDefault: "pi", enabled: []string{"claude", "codex", "pi"}, want: "codex"},
		{name: "SelectedMissingConfigEnabled", selected: "claude", configDefault: "pi", enabled: []string{"codex", "pi"}, want: "pi"},
		{name: "BothMissingFirstSorted", selected: "claude", configDefault: "gemini", enabled: []string{"pi", "codex"}, want: "codex"},
		{name: "NoEngines", selected: "claude", configDefault: "pi", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := enginecfg.NewStore(tc.selected, tc.configDefault, enabled(tc.enabled...))
			assert.Equal(t, tc.want, s.Get())
			assert.Equal(t, tc.selected, s.Selected())
		})
	}
}

func TestStore_GetFollowsEnabledEngines(t *testing.T) {
	ids := []string{"pi"}
	s := enginecfg.NewStore("claude", "", func() []string { return ids })
	assert.Equal(t, "pi", s.Get())

	ids = []string{"claude", "pi"}
	assert.Equal(t, "claude", s.Get())
	assert.Equal(t, "claude", s.Selected())
}

func TestSet(t *testing.T) {
	s := enginecfg.NewStore("claude", "", enabled("claude", "codex"))
	s.Set("codex")
	assert.Equal(t, "codex", s.Get())
	assert.Equal(t, "codex", s.Selected())
}

func TestResolve(t *testing.T) {
	s := enginecfg.NewStore("claude", "", enabled("claude"))
	assert.Equal(t, "codex", s.Resolve("codex"))
	assert.Equal(t, "claude", s.Resolve(""))
}

func TestConcurrentAccess(t *testing.T) {
	s := enginecfg.NewStore("claude", "", enabled("claude", "codex"))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Set("codex") }()
		go func() { defer wg.Done(); _ = s.Get() }()
	}
	wg.Wait()
}
