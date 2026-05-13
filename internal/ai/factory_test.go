package ai_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Test stubs registered at init time. Each test uses a unique engine
// name so we never trip the RegisterEngine duplicate-name panic.
func init() {
	ai.RegisterEngine("factory-test-a", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &factoryStubEngine{name: "factory-test-a"}, nil
	})
	ai.RegisterEngine("factory-test-b", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &factoryStubEngine{name: "factory-test-b"}, nil
	})
	ai.RegisterEngine("factory-test-fail", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return nil, errors.New("boom")
	})
}

type factoryStubEngine struct{ name string }

func (s *factoryStubEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	name := s.name
	return ai.RunResult{ExtractResult: func() string { return name + "-result" }}, errors.New(name + " run called")
}
func (s *factoryStubEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func enabledForTest(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(name string) bool { _, ok := set[name]; return ok }
}

func TestFactory_BuildOnlyConstructsEnabled(t *testing.T) {
	f := ai.NewFactory()
	err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := f.Get("factory-test-a"); !ok {
		t.Error("expected factory-test-a to be built")
	}
	if _, ok := f.Get("factory-test-b"); ok {
		t.Error("expected factory-test-b to be skipped")
	}
}

func TestFactory_BuildPropagatesError(t *testing.T) {
	f := ai.NewFactory()
	err := f.Build(enabledForTest("factory-test-fail"), func(string) map[string]any { return nil })
	if err == nil {
		t.Fatal("expected Build to return error")
	}
	want := `init engine "factory-test-fail": boom`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestFactory_GetUnknownReturnsFalse(t *testing.T) {
	f := ai.NewFactory()
	if _, ok := f.Get("does-not-exist"); ok {
		t.Error("expected Get to return false for unknown engine")
	}
}

func TestFactory_EnabledReturnsCopy(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := f.Enabled()
	delete(m, "factory-test-a")
	if _, ok := f.Get("factory-test-a"); !ok {
		t.Error("mutating Enabled() result must not affect Factory state")
	}
}

func TestFactory_NamesIncludesAllRegistrations(t *testing.T) {
	f := ai.NewFactory()
	names := f.Names()
	if !slices.Contains(names, "factory-test-a") || !slices.Contains(names, "factory-test-b") {
		t.Errorf("Names() = %v, expected to include factory-test-a and factory-test-b", names)
	}
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := f.Names(); !slices.Contains(got, "factory-test-b") {
		t.Errorf("Names() after Build = %v, expected factory-test-b still present", got)
	}
}

func TestResolveExtraArgs_SingleLayer(t *testing.T) {
	layer := `{"claude": "--model sonnet --verbose"}`
	got := ai.ResolveExtraArgs("claude", layer)
	if want := "--model sonnet --verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_MergesLayersInOrder(t *testing.T) {
	base := `{"claude": "--model sonnet --verbose"}`
	override := `{"claude": "--model opus"}`
	got := ai.ResolveExtraArgs("claude", base, override)
	if want := "--model sonnet --verbose --model opus"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_MissingEngineReturnsEmpty(t *testing.T) {
	layer := `{"codex": "--model o3"}`
	if got := ai.ResolveExtraArgs("claude", layer); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestResolveExtraArgs_SkipsEmptyLayers(t *testing.T) {
	got := ai.ResolveExtraArgs("claude", "", "{}", `{"claude":"--model opus"}`)
	if want := "--model opus"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_SkipsMalformedJSON(t *testing.T) {
	got := ai.ResolveExtraArgs("claude", `{not json`, `{"claude":"--verbose"}`)
	if want := "--verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_PreservesQuotingInValue(t *testing.T) {
	layer := `{"claude": "--append-system-prompt \"be terse\" --verbose"}`
	got := ai.ResolveExtraArgs("claude", layer)
	if want := `--append-system-prompt "be terse" --verbose`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_SkipsEmptyEngineValue(t *testing.T) {
	got := ai.ResolveExtraArgs("claude", `{"claude":""}`, `{"claude":"--verbose"}`)
	if want := "--verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidateExtraArgs_OK(t *testing.T) {
	if err := ai.ValidateExtraArgs(`--model sonnet --verbose --msg "hi there"`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateExtraArgs_Empty(t *testing.T) {
	if err := ai.ValidateExtraArgs(""); err != nil {
		t.Errorf("empty string should validate, got %v", err)
	}
}

func TestValidateExtraArgs_UnterminatedQuote(t *testing.T) {
	err := ai.ValidateExtraArgs(`--model "unterminated`)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
