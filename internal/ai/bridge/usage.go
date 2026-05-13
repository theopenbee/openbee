package bridge

import (
	"context"
	"errors"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Usage holds per-model token consumption for one session turn. Mirrors
// ai.TokenUsage so business code never touches ai types.
type Usage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// ErrSessionDataNotFound is returned by CollectUsage when no engine-side
// data exists for the (engine, session) pair.
var ErrSessionDataNotFound = errors.New("bridge: session data not found")

// CollectUsage returns per-model usage for the (engineName, sessionID)
// pair. Errors are translated so business code never sees an ai-package
// sentinel.
func (b *bridgeImpl) CollectUsage(ctx context.Context, engineName, sessionID string) ([]Usage, error) {
	eng, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}
	raw, err := eng.CollectTokenUsage(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrSessionDataNotFound, err.Error())
		}
		return nil, err
	}
	out := make([]Usage, len(raw))
	for i, u := range raw {
		out[i] = Usage(u)
	}
	return out, nil
}
