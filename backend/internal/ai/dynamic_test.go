package ai_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestDynamicAdapter_NoEngine(t *testing.T) {
	src := aitest.NewSource(nil)
	d := ai.NewDynamicAdapter(src, enginecfg.NewStore("claude", "pi", src.IDs))

	_, err := d.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	require.ErrorIs(t, err, ai.ErrNoEngine)
	assert.EqualError(t, err, "no engine is installed or enabled")
}

func TestDynamicAdapter_RoutesToEffectiveDefault(t *testing.T) {
	pi := &aitest.Engine{}
	codex := &aitest.Engine{}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi, "codex": codex})
	cfg := enginecfg.NewStore("claude", "pi", src.IDs)
	d := ai.NewDynamicAdapter(src, cfg)
	run := func() {
		t.Helper()
		_, err := d.Run(context.Background(), "/w", "p", ai.RunOptions{Role: ai.RoleBee}, "/log")
		require.NoError(t, err)
	}

	run()
	assert.Len(t, pi.Runs(), 1)
	assert.Empty(t, codex.Runs())
	assert.Equal(t, ai.RoleBee, pi.Runs()[0].Role)

	cfg.Set("codex")
	run()
	assert.Len(t, codex.Runs(), 1)

	src.Remove("codex")
	run()
	assert.Len(t, pi.Runs(), 2)
	assert.Equal(t, "codex", cfg.Selected())
}

func TestDynamicAdapter_RunBindsExtractResultToEngine(t *testing.T) {
	engine := func(name string) *aitest.Engine {
		return &aitest.Engine{RunFunc: func(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
			return ai.RunResult{ExtractResult: func(string) string { return name + "-result" }}, nil
		}}
	}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"a": engine("a"), "b": engine("b")})
	cfg := enginecfg.NewStore("a", "", src.IDs)
	d := ai.NewDynamicAdapter(src, cfg)

	res, err := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	require.NoError(t, err)
	cfg.Set("b")

	assert.Equal(t, "a-result", res.ExtractResult("/log"))
}

func TestIsEngineMissing(t *testing.T) {
	assert.EqualError(t, ai.ErrEngineUnavailable("codex"), `engine "codex" is not installed or enabled`)
	assert.True(t, ai.IsEngineMissing(fmt.Errorf("start runtime: %w", ai.ErrEngineUnavailable("codex"))))
	assert.True(t, ai.IsEngineMissing(fmt.Errorf("start runtime: %w", ai.ErrNoEngine)))
	assert.False(t, ai.IsEngineMissing(errors.New("session broken")))
}
