package core

import (
	"sync"

	"github.com/theopenbee/openbee/internal/ai"
)

// NewRunResult builds an ai.RunResult, propagating err unchanged. The
// provided extract function is wrapped with sync.Once so ExtractResult
// only runs the underlying scan the first time and returns the cached
// result thereafter.
func NewRunResult(proc ai.Process, out <-chan ai.Output, err error, extract func() string) (ai.RunResult, error) {
	if err != nil {
		return ai.RunResult{}, err
	}
	var (
		once   sync.Once
		result string
	)
	memo := func() string {
		once.Do(func() { result = extract() })
		return result
	}
	return ai.RunResult{Process: proc, Output: out, ExtractResult: memo}, nil
}
