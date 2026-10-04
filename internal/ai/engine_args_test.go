package ai_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestParseEngineArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	raw := map[string]string{
		"claude": `--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	require.NoError(t, err)

	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	require.Equal(t, want, got["claude"])
}

func TestParseEngineArgs_PreservesDuplicateFlags(t *testing.T) {
	raw := map[string]string{
		"codex": `--include src --include test`,
	}
	got, err := ai.ParseEngineArgs(raw)
	require.NoError(t, err)

	want := []string{"--include", "src", "--include", "test"}
	require.Equal(t, want, got["codex"])
}

func TestParseEngineArgs_PreservesEmptyQuotedValue(t *testing.T) {
	raw := map[string]string{
		"claude": `--append-system-prompt "" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	require.NoError(t, err)

	want := []string{"--append-system-prompt", "", "--verbose"}
	require.Equal(t, want, got["claude"])
}

func TestParseEngineArgs_UnterminatedQuote(t *testing.T) {
	_, err := ai.ParseEngineArgs(map[string]string{
		"claude": `--model "unterminated`,
	})
	require.Error(t, err)
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

	wantClaude := []string{"--model", "sonnet", "--verbose", "--model", "opus"}
	require.Equal(t, wantClaude, got["claude"])
	wantCodex := []string{"--model", "o3"}
	require.Equal(t, wantCodex, got["codex"])
}
