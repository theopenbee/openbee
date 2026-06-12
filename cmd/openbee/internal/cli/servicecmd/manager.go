package servicecmd

import "context"

// RunState reports the service manager's view of the underlying process.
type RunState int

const (
	RunStateUnknown RunState = iota
	RunStateStopped
	RunStateRunning
	RunStateFailed
)

func (s RunState) String() string {
	switch s {
	case RunStateStopped:
		return "stopped"
	case RunStateRunning:
		return "running"
	case RunStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Status is the result of Manager.Status.
type Status struct {
	Installed  bool
	RunState   RunState
	PID        int
	UptimeSecs int64
}

// InstallOptions captures everything Manager.Install needs to render and
// register a platform-specific autostart artifact.
type InstallOptions struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	AutoStart  bool
	Force      bool
}

// Manager is the platform-neutral autostart abstraction.
type Manager interface {
	Install(ctx context.Context, opts InstallOptions) error
	Uninstall(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}
