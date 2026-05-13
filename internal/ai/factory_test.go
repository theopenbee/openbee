package ai_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
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

func TestFactory_DynamicRoutesToCurrentEngine(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a", "factory-test-b"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("factory-test-a")
	dyn := f.Dynamic(cfg)

	_, err := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "factory-test-a run called" {
		t.Errorf("expected 'factory-test-a run called', got %v", err)
	}

	cfg.Set("factory-test-b")
	_, err = dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "factory-test-b run called" {
		t.Errorf("expected 'factory-test-b run called', got %v", err)
	}
}

func TestFactory_DynamicBindsExtractToRunTimeEngine(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a", "factory-test-b"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("factory-test-a")
	dyn := f.Dynamic(cfg)

	res, _ := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	cfg.Set("factory-test-b") // simulate /engine switch mid-execution
	if got := res.ExtractResult(); got != "factory-test-a-result" {
		t.Errorf("expected run-time engine extractor; got %s", got)
	}
}

func TestFactory_DynamicUnknownEngineErrors(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("not-built")
	dyn := f.Dynamic(cfg)
	_, err := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
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
