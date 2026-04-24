package ai_test

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestParseEngineExtraArgs(t *testing.T) {
	raw := map[string]string{
		"claude": "--model claude-sonnet-4-5 --effort high",
		"codex":  "--model o3",
	}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["claude"]["model"] != "claude-sonnet-4-5" {
		t.Errorf("claude model: got %q", got["claude"]["model"])
	}
	if got["claude"]["effort"] != "high" {
		t.Errorf("claude effort: got %q", got["claude"]["effort"])
	}
	if got["codex"]["model"] != "o3" {
		t.Errorf("codex model: got %q", got["codex"]["model"])
	}
}

func TestParseEngineExtraArgs_BooleanFlag(t *testing.T) {
	raw := map[string]string{"claude": "--verbose"}
	got, err := ai.ParseEngineExtraArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := got["claude"]["verbose"]
	if !ok {
		t.Fatal("verbose key missing")
	}
	if v != "" {
		t.Errorf("verbose value should be empty, got %q", v)
	}
}

func TestMergeEngineExtraArgs(t *testing.T) {
	base := ai.EngineExtraArgsMap{
		"claude": {"model": "sonnet", "effort": "high"},
	}
	override := ai.EngineExtraArgsMap{
		"claude": {"model": "opus"},
		"codex":  {"model": "o3"},
	}
	got := ai.MergeEngineExtraArgs(base, override)
	if got["claude"]["model"] != "opus" {
		t.Errorf("expected opus, got %q", got["claude"]["model"])
	}
	if got["claude"]["effort"] != "high" {
		t.Errorf("effort should be inherited: got %q", got["claude"]["effort"])
	}
	if got["codex"]["model"] != "o3" {
		t.Errorf("codex model: got %q", got["codex"]["model"])
	}
}

func TestBuildExtraArgSlice(t *testing.T) {
	args := map[string]string{"model": "claude-sonnet-4-5", "verbose": ""}
	slice := ai.BuildExtraArgSlice(args)
	found := map[string]bool{}
	for i := 0; i < len(slice); i++ {
		found[slice[i]] = true
	}
	if !found["--model"] {
		t.Error("missing --model")
	}
	if !found["claude-sonnet-4-5"] {
		t.Error("missing model value")
	}
	if !found["--verbose"] {
		t.Error("missing --verbose")
	}
}
