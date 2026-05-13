package adapters

import (
	"time"

	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
)

// execLogPathPreparer is the subset of *store.ExecutionStore used here.
type execLogPathPreparer interface {
	PrepareLogPath(executionID string, startedAt time.Time) (string, error)
}

type logPathProvider struct{ store execLogPathPreparer }

func NewLogPathProvider(store execLogPathPreparer) bridge.LogPathProvider {
	return logPathProvider{store: store}
}

func (l logPathProvider) PrepareForWorker(executionID string, startedAt time.Time) (string, error) {
	return l.store.PrepareLogPath(executionID, startedAt)
}
