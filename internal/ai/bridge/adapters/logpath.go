package adapters

import (
	"time"

	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
)

// execLogPathPreparer is the subset of *store.ExecutionStore used here.
// The store expects startedAt as *int64 milliseconds since epoch; the
// adapter converts the bridge port's time.Time into that shape.
type execLogPathPreparer interface {
	PrepareLogPath(id string, startedAt *int64) (string, error)
}

type logPathProvider struct{ store execLogPathPreparer }

func NewLogPathProvider(store execLogPathPreparer) bridge.LogPathProvider {
	return logPathProvider{store: store}
}

func (l logPathProvider) PrepareForWorker(executionID string, startedAt time.Time) (string, error) {
	var ms *int64
	if !startedAt.IsZero() {
		v := startedAt.UnixMilli()
		ms = &v
	}
	return l.store.PrepareLogPath(executionID, ms)
}
