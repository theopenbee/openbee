package bridge

import (
	"context"
	"time"
)

// Status classifies the terminal state of a run.
type Status int

const (
	StatusCompleted Status = iota + 1
	StatusFailed
	StatusAbandoned // process exited without a Done/Error signal
)

// Outcome is the terminal result of a run.
type Outcome struct {
	Status Status
	Result string
}

// Handle is the lifecycle handle for a started run.
type Handle interface {
	PID() int
	EngineName() string
	Stop() error
	Wait(ctx context.Context) (Outcome, error)
}

// WorkerRunRequest carries the inputs required to run a worker.
type WorkerRunRequest struct {
	WorkerID         string
	PermissionScopes []string
	ExecutionID      string
	StartedAt        time.Time
	EngineHint       string
	EngineArgs       string
	WorkDir          string
	Prompt           string
	SessionID        string
	Resume           bool
	Timeout          time.Duration
}

// BeeRunRequest carries the inputs required to run a bee.
type BeeRunRequest struct {
	WorkDir   string
	Prompt    string
	SessionID string
	Resume    bool
	LogPath   string
}
