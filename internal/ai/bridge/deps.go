package bridge

import (
	"context"
	"time"
)

// TokenIssuer mints short-lived auth tokens for engine invocations.
type TokenIssuer interface {
	WorkerToken(workerID string, scopes []string) (string, error)
	BeeToken() (string, error)
}

// EnvResolver returns the KEY=VALUE env list to inject for a given role.
type EnvResolver interface {
	WorkerEnv(workerID string) ([]string, error)
	BeeEnv() ([]string, error)
}

// EngineSelector picks the engine name for a given role and hint.
type EngineSelector interface {
	// ForWorker returns hint when it names an enabled engine, otherwise
	// the current default.
	ForWorker(hint string) string
	// ForBee returns the current default engine.
	ForBee() string
}

// ArgsResolver merges global + role-specific engine_args JSON layers and
// returns the raw CLI tail for engineName. Failures upstream (missing
// rows, malformed JSON) are silently treated as empty layers; this
// matches existing behaviour and ensures a corrupt config row cannot
// block a run.
type ArgsResolver interface {
	ForWorker(ctx context.Context, workerEngineArgs, engineName string) string
	ForBee(ctx context.Context, engineName string) string
}

// LogPathProvider prepares the on-disk log path for a worker execution.
type LogPathProvider interface {
	PrepareForWorker(executionID string, startedAt time.Time) (string, error)
}
