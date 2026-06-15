package servicecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// execLookPath is overridden in tests to simulate missing tools.
var execLookPath = exec.LookPath

// runAsLookupTimeout bounds the runuser calls so a stuck profile script can't
// hang `service install`. 5s is generous for a login shell + printf.
var runAsLookupTimeout = 5 * time.Second

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
	// EnvPath is the PATH value embedded into the launchd plist. launchd starts
	// jobs with a minimal default PATH that excludes `/usr/local/bin`,
	// `/opt/homebrew/bin`, nvm directories, etc., so node-based CLIs (claude,
	// codex) fail with `env: node: No such file or directory` unless we
	// forward the install-time PATH. The Linux backend does NOT bake this into
	// the unit — it wraps ExecStart in `/bin/bash -ilc` and reads PATH from the
	// run-as user's interactive-login profile at start time. EnvPath is still
	// computed on Linux so install-time warnings (node missing / not
	// executable) have a value to display.
	EnvPath   string
	AutoStart bool
	Force     bool
	// RunAsUser / RunAsGroup are only consumed by the Linux backend, where the
	// system-wide systemd unit needs explicit User=/Group= directives so the
	// daemon does not inherit root from the installing sudo invocation. Empty
	// on darwin/windows.
	RunAsUser  string
	RunAsGroup string
	// Home is the HOME the daemon should see. On Linux this is the RunAsUser's
	// home (so the daemon does not inherit /root from sudo); on darwin/windows
	// it is the installing user's home.
	Home string
}

type Manager interface {
	Install(ctx context.Context, opts InstallOptions) error
	Uninstall(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

// userLookup is overridden in tests.
var userLookup = user.Lookup

func resolveInstallOptions(configFlag, workingDirFlag, runAsFlag string, noStart, force bool) (InstallOptions, []string, error) {
	exe, err := utils.ResolveExecutable()
	if err != nil {
		return InstallOptions{}, nil, fmt.Errorf("resolve executable: %w", err)
	}

	runAsUser, runAsGroup, targetHome, err := resolveRunAs(runAsFlag)
	if err != nil {
		return InstallOptions{}, nil, err
	}

	openbeeHome := filepath.Join(targetHome, ".openbee")

	cfgPath := configFlag
	if cfgPath == "" {
		cfgPath = filepath.Join(openbeeHome, "config.yaml")
	}
	cfgInfo, err := os.Stat(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstallOptions{}, nil, fmt.Errorf(i18n.M.Output.Service.ConfigMissing, cfgPath)
		}
		return InstallOptions{}, nil, fmt.Errorf("stat config: %w", err)
	}
	if cfgInfo.IsDir() {
		return InstallOptions{}, nil, fmt.Errorf(
			i18n.M.Output.Service.ConfigPathIsDir,
			cfgPath,
			filepath.Join(cfgPath, "config.yaml"),
		)
	}

	logPath := filepath.Join(openbeeHome, "openbee.log")

	workingDir := workingDirFlag
	if workingDir == "" {
		workingDir = openbeeHome
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

	envPath, pathWarning := resolveEnvPath(runAsUser)
	var warnings []string
	if pathWarning != "" {
		warnings = append(warnings, pathWarning)
	}
	warnings = appendNodeWarning(warnings, runAsUser, envPath)

	return InstallOptions{
		ExePath:    exe,
		ConfigPath: cfgPath,
		LogPath:    logPath,
		WorkingDir: workingDir,
		EnvPath:    envPath,
		AutoStart:  !noStart,
		Force:      force,
		RunAsUser:  runAsUser,
		RunAsGroup: runAsGroup,
		Home:       targetHome,
	}, warnings, nil
}

// resolveRunAs picks the user the daemon will run as. On non-Linux platforms
// run-as is meaningless (launchd / Task Scheduler bind to the calling user
// anyway), so we just resolve the current user's home for default-path
// computation and leave RunAsUser/Group empty.
func resolveRunAs(runAsFlag string) (string, string, string, error) {
	if runtime.GOOS != "linux" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", "", fmt.Errorf("resolve home dir: %w", err)
		}
		return "", "", home, nil
	}
	name := runAsFlag
	if name == "" {
		name = os.Getenv("SUDO_USER")
	}
	if name == "" {
		return "", "", "", errors.New(i18n.M.Output.Service.RunAsRequired)
	}
	u, err := userLookup(name)
	if err != nil {
		return "", "", "", fmt.Errorf(i18n.M.Output.Service.RunAsUserUnknown, name)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		// Group name lookup can fail in minimal containers where /etc/group is
		// sparse but a valid GID still works in systemd unit Group= directive.
		return u.Username, u.Gid, u.HomeDir, nil
	}
	return u.Username, g.Name, u.HomeDir, nil
}

var newManager = NewManager

// resolveEnvPath decides which PATH to bake into the service unit. When the
// daemon runs as a different user than the installer (Linux + RunAsUser), the
// installer's PATH is the wrong answer — it often points at /root/.nvm or
// sudo's secure_path, both of which the daemon user cannot use. We prefer the
// run-as user's own login PATH and only fall back when we can't reach it.
func resolveEnvPath(runAsUser string) (string, string) {
	installerPath := os.Getenv("PATH")
	if runAsUser == "" {
		return installerPath, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), runAsLookupTimeout)
	defer cancel()
	userPath, err := lookupRunAsEnvPath(ctx, runAsUser)
	if err != nil {
		return installerPath, fmt.Sprintf(i18n.M.Output.Service.RunAsPathResolveFailed, runAsUser, err)
	}
	if userPath == "" {
		return installerPath, ""
	}
	return userPath, ""
}

// appendNodeWarning runs the node-availability check the daemon would face at
// runtime and appends the right warning. We split missing vs not-executable
// because the fix differs: install node vs. install it for a *different* user
// (or chmod +x). When verification can't run (non-linux, helper stubbed) we
// fall back to the cheap execLookPath probe so behaviour stays at parity.
func appendNodeWarning(warnings []string, runAsUser, envPath string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), runAsLookupTimeout)
	defer cancel()
	switch verifyNodeForRunAsUser(ctx, runAsUser, envPath) {
	case nodeCheckOK:
		return warnings
	case nodeCheckMissing:
		return append(warnings, fmt.Sprintf(i18n.M.Output.Service.NodeMissingWarning, runAsUser, envPath))
	case nodeCheckNotExecutable:
		return append(warnings, fmt.Sprintf(i18n.M.Output.Service.NodeNotExecutableWarning, runAsUser, envPath))
	}
	if _, err := execLookPath("node"); err != nil {
		return append(warnings, fmt.Sprintf(i18n.M.Output.Service.NodeMissingWarning, runAsUser, envPath))
	}
	return warnings
}
