package ai_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// stubEngine is a minimal EngineAdapter for testing.
type stubEngine struct {
	name string
}

func (s *stubEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	name := s.name
	return ai.RunResult{
		ExtractResult: func() string { return name + "-result" },
	}, errors.New(s.name + " run called")
}
func (s *stubEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func TestParseEngineArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	raw := map[string]string{
		"claude": `--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_PreservesDuplicateFlags(t *testing.T) {
	raw := map[string]string{
		"codex": `--include src --include test`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got["codex"], want) {
		t.Fatalf("got %v, want %v", got["codex"], want)
	}
}

func TestParseEngineArgs_PreservesEmptyQuotedValue(t *testing.T) {
	raw := map[string]string{
		"claude": `--append-system-prompt "" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_UnterminatedQuote(t *testing.T) {
	_, err := ai.ParseEngineArgs(map[string]string{
		"claude": `--model "unterminated`,
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestMergeEngineArgs_AppendsOverrideArgs(t *testing.T) {
	base := ai.EngineArgsMap{
		"claude": {"--model", "sonnet", "--verbose"},
	}
	override := ai.EngineArgsMap{
		"claude": {"--model", "opus"},
		"codex":  {"--model", "o3"},
	}
	got := ai.MergeEngineArgs(base, override)

	if want := []string{"--model", "sonnet", "--verbose", "--model", "opus"}; !slices.Equal(got["claude"], want) {
		t.Fatalf("claude args = %v, want %v", got["claude"], want)
	}
	if want := []string{"--model", "o3"}; !slices.Equal(got["codex"], want) {
		t.Fatalf("codex args = %v, want %v", got["codex"], want)
	}
}

// stubAdapter is a no-op EngineAdapter for registry tests.
type stubAdapter struct{}

func (s *stubAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	return ai.RunResult{ExtractResult: func() string { return "" }}, nil
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
