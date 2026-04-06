// internal/ai/registry_test.go
package ai_test

import (
	"context"
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// stubAdapter is a no-op EngineAdapter for registry tests.
type stubAdapter struct{}

func (s *stubAdapter) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
	return nil
}
func (s *stubAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
	return nil, nil, nil
}

func TestRegistry_NewReturnsRegisteredEngine(t *testing.T) {
	r := ai.NewRegistry()
	r.Register("stub", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &stubAdapter{}, nil
	})
	eng, err := r.New("stub", ai.EngineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng == nil {
		t.Error("expected non-nil adapter")
	}
}

func TestRegistry_NewUnknownEngineReturnsError(t *testing.T) {
	r := ai.NewRegistry()
	_, err := r.New("unknown", ai.EngineConfig{})
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
	if !errors.Is(err, ai.ErrUnknownEngine) {
		t.Errorf("expected ErrUnknownEngine, got: %v", err)
	}
}

func TestRegistry_NewCallsFactory(t *testing.T) {
	r := ai.NewRegistry()
	called := false
	r.Register("called", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		called = true
		return &stubAdapter{}, nil
	})
	r.New("called", ai.EngineConfig{})
	if !called {
		t.Error("factory was not called")
	}
}
