package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

// bridgeImpl is the concrete Bridge. Methods are split across files for
// readability (names.go / validate.go / usage.go / run.go).
type bridgeImpl struct {
	engines map[string]ai.EngineAdapter
}
