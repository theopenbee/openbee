package core

import (
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
)

func TestNewRunResult_MemoizesExtract(t *testing.T) {
	calls := 0
	res, err := NewRunResult(nil, nil, nil, func() string {
		calls++
		return "value"
	})
	if err != nil {
		t.Fatalf("NewRunResult: %v", err)
	}
	for i := range 3 {
		if got := res.ExtractResult(); got != "value" {
			t.Fatalf("call %d: got %q want %q", i, got, "value")
		}
	}
	if calls != 1 {
		t.Errorf("extract should run once; got %d calls", calls)
	}
}

func TestNewRunResult_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := NewRunResult(nil, nil, wantErr, func() string { return "" })
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

// Ensure ai package import is used so the test file compiles even if
// future edits remove the only ai.* reference.
var _ ai.Process = (ai.Process)(nil)
