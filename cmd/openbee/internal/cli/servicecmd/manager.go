package servicecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// lookPathInstall is overridden in tests to simulate missing tools.
var lookPathInstall = exec.LookPath

// RunStateFailed is only produced by the linux (systemd) backend, which
// surfaces a distinct "failed" ActiveState; launchd and Task Scheduler collapse
// failure into RunStateStopped.
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
	Installed      bool
	RunState       RunState
	PID            int
	LastExitCode   string
	LastExitReason string
}

type InstallOptions struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	WorkingDir string
	// EnvPath is the PATH value that should be embedded into the service unit
	// (launchd plist / systemd unit). launchd and systemd start jobs with a
	// minimal default PATH that excludes `/usr/local/bin`, `/opt/homebrew/bin`,
	// nvm directories, etc., so node-based CLIs (claude, codex) fail with
	// `env: node: No such file or directory` unless we explicitly forward
	// the install-time PATH.
	EnvPath   string
	AutoStart bool
	Force     bool
}

type Manager interface {
	Install(ctx context.Context, opts InstallOptions) error
	Uninstall(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

func resolveInstallOptions(configFlag, workingDirFlag string, noStart, force bool) (InstallOptions, []string, error) {
	exe, err := utils.ResolveExecutable()
	if err != nil {
		return InstallOptions{}, nil, fmt.Errorf("resolve executable: %w", err)
	}

	cfgPath := configFlag
	if cfgPath == "" {
		home, err := config.OpenbeeHomeDir()
		if err != nil {
			return InstallOptions{}, nil, fmt.Errorf("resolve home dir: %w", err)
		}
		cfgPath = filepath.Join(home, "config.yaml")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstallOptions{}, nil, fmt.Errorf(i18n.M.Output.Service.ConfigMissing, cfgPath)
		}
		return InstallOptions{}, nil, fmt.Errorf("stat config: %w", err)
	}

	logPath, err := config.DaemonLogFile()
	if err != nil {
		return InstallOptions{}, nil, fmt.Errorf("resolve log path: %w", err)
	}

	workingDir := workingDirFlag
	if workingDir == "" {
		workingDir, err = config.OpenbeeHomeDir()
		if err != nil {
			return InstallOptions{}, nil, fmt.Errorf("resolve working dir: %w", err)
		}
	}
	if !filepath.IsAbs(workingDir) {
		abs, err := filepath.Abs(workingDir)
		if err != nil {
			return InstallOptions{}, nil, fmt.Errorf("resolve working dir: %w", err)
		}
		workingDir = abs
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return InstallOptions{}, nil, fmt.Errorf("create working dir: %w", err)
	}

	envPath := os.Getenv("PATH")
	var warnings []string
	if _, err := lookPathInstall("node"); err != nil {
		warnings = append(warnings, fmt.Sprintf(i18n.M.Output.Service.NodeMissingWarning, envPath))
	}

	return InstallOptions{
		ExePath:    exe,
		ConfigPath: cfgPath,
		LogPath:    logPath,
		WorkingDir: workingDir,
		EnvPath:    envPath,
		AutoStart:  !noStart,
		Force:      force,
	}, warnings, nil
}

var newManager = NewManager
