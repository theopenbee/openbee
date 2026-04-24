package ai_test

import (
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestParseEngineExtraArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	raw := map[string]string{
		"claude": `--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`,
	}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineExtraArgs_PreservesDuplicateFlags(t *testing.T) {
	raw := map[string]string{
		"codex": `--include src --include test`,
	}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got["codex"], want) {
		t.Fatalf("got %v, want %v", got["codex"], want)
	}
}

func TestParseEngineExtraArgs_PreservesEmptyQuotedValue(t *testing.T) {
	raw := map[string]string{
		"claude": `--append-system-prompt "" --verbose`,
	}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineExtraArgs_UnterminatedQuote(t *testing.T) {
	_, err := ai.ParseEngineExtraArgs(map[string]string{
		"claude": `--model "unterminated`,
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestMergeEngineExtraArgs_AppendsOverrideArgs(t *testing.T) {
	base := ai.EngineExtraArgsMap{
		"claude": {"--model", "sonnet", "--verbose"},
	}
	override := ai.EngineExtraArgsMap{
		"claude": {"--model", "opus"},
		"codex":  {"--model", "o3"},
	}
	got := ai.MergeEngineExtraArgs(base, override)

	if want := []string{"--model", "sonnet", "--verbose", "--model", "opus"}; !slices.Equal(got["claude"], want) {
		t.Fatalf("claude args = %v, want %v", got["claude"], want)
	}
	if want := []string{"--model", "o3"}; !slices.Equal(got["codex"], want) {
		t.Fatalf("codex args = %v, want %v", got["codex"], want)
	}
}

func TestBuildExtraArgSlice(t *testing.T) {
	args := []string{"--model", "claude-sonnet-4-5", "--verbose"}
	slice := ai.BuildExtraArgSlice(args)
	if !slices.Equal(slice, args) {
		t.Fatalf("got %v, want %v", slice, args)
	}
	if len(slice) > 0 {
		slice[0] = "--changed"
	}
	if args[0] != "--model" {
		t.Fatalf("BuildExtraArgSlice should return a copy, args mutated to %v", args)
	}
}
