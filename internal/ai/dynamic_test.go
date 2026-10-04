package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// stubEngine is a minimal EngineAdapter for testing.
type stubEngine struct {
	name     string
	prepared []string // workDirs seen
}

func (s *stubEngine) Prepare(workDir string, _ ai.PrepareOptions) error {
	s.prepared = append(s.prepared, workDir)
	return nil
}
func (s *stubEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	name := s.name
	return ai.RunResult{
		ExtractResult: func(string) string { return name + "-result" },
	}, errors.New(s.name + " run called")
}
func (s *stubEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func TestDynamicAdapter_PrepareCallsAll(t *testing.T) {
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	cfg := enginecfg.NewStore("a")
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)
	require.NoError(t, d.Prepare("/work", ai.PrepareOptions{}))
	assert.Len(t, a.prepared, 1)
	assert.Len(t, b.prepared, 1)
}

func TestDynamicAdapter_RunRoutesToCurrentEngine(t *testing.T) {
	cfg := enginecfg.NewStore("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)

	_, err := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	require.Error(t, err) // guard: err.Error() below would panic if err were nil
	assert.Equal(t, "a run called", err.Error())

	cfg.Set("b")
	_, err = d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	require.Error(t, err) // guard: err.Error() below would panic if err were nil
	assert.Equal(t, "b run called", err.Error())
}

func TestDynamicAdapter_RunBindsExtractResultToEngine(t *testing.T) {
	cfg := enginecfg.NewStore("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)

	res, _ := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")

	// Simulate /engine switch mid-execution.
	cfg.Set("b")

	got := res.ExtractResult("/log")
	assert.Equal(t, "a-result", got)
}

func TestDynamicAdapter_RunUnknownEngine(t *testing.T) {
	cfg := enginecfg.NewStore("missing")
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": &stubEngine{name: "a"}}, cfg)
	_, err := d.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	assert.Error(t, err)
}
