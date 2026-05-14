package bridge

import (
	"context"
	"errors"
)

const (
	EngineClaude = "claude"
	EngineCodex  = "codex"
	EnginePi     = "pi"
	EngineKimi   = "kimi"
)

var canonicalEngines = []string{EngineClaude, EngineCodex, EnginePi, EngineKimi}

var ErrSessionDataNotFound = errors.New("bridge: session data not found")

func AllEngines() []string {
	cp := make([]string, len(canonicalEngines))
	copy(cp, canonicalEngines)
	return cp
}

type Bridge interface {
	EnabledEngines() []string
	ValidateEngine(name string) error
	ResolveEngine(workerEngine string) (string, error)

	BuildBeeSessionPrefix() string
	BuildWorkerSessionPrefix(persona WorkerPersona) string

	PrepareBeeWorkspace(workDir string) error
	PrepareWorkerWorkspace(workDir string, engineName string) error

	RunBee(ctx context.Context, req BeeRunRequest) (RunHandle, error)
	RunWorker(ctx context.Context, req WorkerRunRequest) (RunHandle, error)

	CollectTokenUsage(ctx context.Context, sessionID, engineName string) (UsageResult, error)
}

type WorkerPersona struct {
	Name        string
	Description string
	Constraints string
}

type BeeRunRequest struct {
	WorkDir   string
	Engine    string
	Prompt    string
	SessionID string
	Resume    bool
	LogPath   string
}

type WorkerRunRequest struct {
	WorkerID         string
	WorkDir          string
	PermissionScopes []string
	WorkerEngine     string
	WorkerEngineArgs string
	Prompt           string
	SessionID        string
	Resume           bool
	LogPath          string
}

type RunHandle struct {
	Engine        string
	Process       ProcessHandle
	Events        <-chan LifecycleEvent
	ExtractResult func(logPath string) string
}

type ProcessHandle interface {
	PID() int
	Stop() error
}

type LifecycleEventType string

const (
	LifecycleDone  LifecycleEventType = "done"
	LifecycleError LifecycleEventType = "error"
)

type LifecycleEvent struct {
	Type    LifecycleEventType
	Content string
}

type TokenUsage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

type UsageResult struct {
	Engine string
	Usages []TokenUsage
}
