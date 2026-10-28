// internal/ai/registry_test.go
package ai_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// stubAdapter is a no-op EngineAdapter for registry tests.
type stubAdapter struct{}

func (s *stubAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}
func (s *stubAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	return ai.RunResult{ExtractResult: func(string) string { return "" }}, nil
}
func (s *stubAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func TestRegistry_NewReturnsRegisteredEngine(t *testing.T) {
	r := ai.NewRegistry()
	r.Register("stub", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &stubAdapter{}, nil
	})
	eng, err := r.New("stub", ai.EngineConfig{})
	require.NoError(t, err)
	assert.NotNil(t, eng)
}

func TestRegistry_NewUnknownEngineReturnsError(t *testing.T) {
	r := ai.NewRegistry()
	_, err := r.New("unknown", ai.EngineConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ai.ErrUnknownEngine)
}

func TestRegistry_NewCallsFactory(t *testing.T) {
	r := ai.NewRegistry()
	called := false
	r.Register("called", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		called = true
		return &stubAdapter{}, nil
	})
	r.New("called", ai.EngineConfig{})
	assert.True(t, called, "factory was not called")
}
