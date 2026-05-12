package core

import (
	"encoding/json"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

// AggregateUsage scans a JSONL file at path, unmarshals each line as T, and
// lets fold accumulate per-model TokenUsage into agg. Lines that fail to
// unmarshal are silently skipped (matches existing per-engine behavior).
// The returned slice ordering is unspecified.
func AggregateUsage[T any](path string, fold func(line T, agg map[string]*ai.TokenUsage)) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line T
		if json.Unmarshal(data, &line) != nil {
			return
		}
		fold(line, agg)
	})
	if err != nil {
		return nil, err
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
