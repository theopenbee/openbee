package core_test

import (
	"slices"
	"strings"
	"testing"

	core "github.com/theopenbee/openbee/internal/ai/core"
)

func TestSplitArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	got, err := core.SplitArgs(`--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_PreservesDuplicateFlags(t *testing.T) {
	got, err := core.SplitArgs(`--include src --include test`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_PreservesEmptyQuotedValue(t *testing.T) {
	got, err := core.SplitArgs(`--append-system-prompt "" --verbose`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_HandlesSingleQuotes(t *testing.T) {
	got, err := core.SplitArgs(`--msg 'hello world'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--msg", "hello world"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_HandlesBackslashEscape(t *testing.T) {
	got, err := core.SplitArgs(`a\ b c`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a b", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_EmptyStringReturnsNil(t *testing.T) {
	got, err := core.SplitArgs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestSplitArgs_UnterminatedDoubleQuote(t *testing.T) {
	_, err := core.SplitArgs(`--model "unterminated`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "unterminated quoted string") {
		t.Errorf("error = %q, want contains 'unterminated quoted string'", err.Error())
	}
}

func TestSplitArgs_UnterminatedSingleQuote(t *testing.T) {
	_, err := core.SplitArgs(`--msg 'unterminated`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSplitArgs_UnterminatedEscape(t *testing.T) {
	_, err := core.SplitArgs(`--flag value\`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "unterminated escape sequence") {
		t.Errorf("error = %q, want contains 'unterminated escape sequence'", err.Error())
	}
}
