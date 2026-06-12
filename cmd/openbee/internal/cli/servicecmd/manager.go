package servicecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// RunState reports the service manager's view of the underlying process.
//
// RunStateFailed is currently only produced by the linux (systemd) backend,
// which surfaces a distinct "failed" ActiveState; launchd and Task Scheduler
// collapse failure into RunStateStopped.
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

type Status struct {
	Installed  bool
	RunState   RunState
	PID        int
	UptimeSecs int64
}

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

func resolveInstallOptions(configFlag string, noStart, force bool) (InstallOptions, error) {
	exe, err := utils.ResolveExecutable()
	if err != nil {
		return InstallOptions{}, fmt.Errorf("resolve executable: %w", err)
	}

	cfgPath := configFlag
	if cfgPath == "" {
		home, err := config.OpenbeeHomeDir()
		if err != nil {
			return InstallOptions{}, fmt.Errorf("resolve home dir: %w", err)
		}
		cfgPath = filepath.Join(home, "config.yaml")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstallOptions{}, fmt.Errorf(i18n.M.Output.Service.ConfigMissing, cfgPath)
		}
		return InstallOptions{}, fmt.Errorf("stat config: %w", err)
	}

	logPath, err := config.DaemonLogFile()
	if err != nil {
		return InstallOptions{}, fmt.Errorf("resolve log path: %w", err)
	}

	return InstallOptions{
		ExePath:    exe,
		ConfigPath: cfgPath,
		LogPath:    logPath,
		AutoStart:  !noStart,
		Force:      force,
	}, nil
}

// newManager is the package-level factory; tests override it to inject a fake.
var newManager = NewManager
