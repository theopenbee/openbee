# openbee service Autostart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbee service` subcommand group that installs/uninstalls/manages user-level autostart of `openbee server` on macOS (launchd LaunchAgent), Linux (systemd --user unit), and Windows (Task Scheduler).

**Architecture:** A new `cmd/openbee/internal/cli/servicecmd` package exposes 5 cobra subcommands that talk to a platform-neutral `Manager` interface. Three build-tagged files implement `Manager` per OS by rendering an embedded template (plist / unit / task XML) and shelling out to the native service manager (`launchctl` / `systemctl --user` / `schtasks`). All platforms log to `~/.openbee/daemon.log` and reuse `openbee server`'s daemon PID file.

**Tech Stack:** Go 1.x, cobra, `text/template` with `embed.FS`, `os/exec`, existing `internal/infra/config` and `internal/infra/i18n` infrastructure.

**Spec:** `docs/superpowers/specs/2026-06-11-service-autostart-design.md`

---

## File Structure

**New files:**
- `cmd/openbee/internal/cli/servicecmd/command.go` — root `service` cobra command, assembles subcommands
- `cmd/openbee/internal/cli/servicecmd/manager.go` — `Manager` interface, `Status`, `InstallOptions`, `RunState` types, shared helpers
- `cmd/openbee/internal/cli/servicecmd/install.go` — `service install` cobra command + handler
- `cmd/openbee/internal/cli/servicecmd/uninstall.go` — `service uninstall` cobra command + handler
- `cmd/openbee/internal/cli/servicecmd/start.go` — `service start` cobra command + handler
- `cmd/openbee/internal/cli/servicecmd/stop.go` — `service stop` cobra command + handler
- `cmd/openbee/internal/cli/servicecmd/status.go` — `service status` cobra command + handler
- `cmd/openbee/internal/cli/servicecmd/manager_darwin.go` — launchd implementation (build tag `darwin`)
- `cmd/openbee/internal/cli/servicecmd/manager_linux.go` — systemd --user implementation (build tag `linux`)
- `cmd/openbee/internal/cli/servicecmd/manager_windows.go` — Task Scheduler implementation (build tag `windows`)
- `cmd/openbee/internal/cli/servicecmd/manager_other.go` — stub for unsupported platforms (build tag `!darwin && !linux && !windows`)
- `cmd/openbee/internal/cli/servicecmd/templates/launchd.plist.tmpl`
- `cmd/openbee/internal/cli/servicecmd/templates/systemd.service.tmpl`
- `cmd/openbee/internal/cli/servicecmd/templates/schtask.xml.tmpl`
- `cmd/openbee/internal/cli/servicecmd/install_test.go` — command-layer tests with fake Manager
- `cmd/openbee/internal/cli/servicecmd/manager_darwin_test.go` — template snapshot + helper unit tests
- `cmd/openbee/internal/cli/servicecmd/manager_linux_test.go`
- `cmd/openbee/internal/cli/servicecmd/manager_windows_test.go`
- `docs/service-autostart.md` — user-facing documentation

**Modified files:**
- `cmd/openbee/internal/cli/root.go` — register `servicecmd.NewCommand()`
- `internal/infra/i18n/messages.go` — add `CmdService`, `ServiceOutput` structs
- `internal/infra/i18n/locales/en.yaml` — add `cmd.service.*` and `output.service.*` keys
- `internal/infra/i18n/locales/zh.yaml` — same keys in Chinese
- `README.md` — add autostart section pointing to docs
- `CHANGELOG.md` — add Unreleased entry

---

## Task 1: Scaffold `servicecmd` package with Manager interface

**Files:**
- Create: `cmd/openbee/internal/cli/servicecmd/manager.go`
- Create: `cmd/openbee/internal/cli/servicecmd/command.go`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_other.go`

- [ ] **Step 1: Write `manager.go` with interface and types**

```go
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
```

- [ ] **Step 2: Write `manager_other.go` stub**

```go
//go:build !darwin && !linux && !windows

package servicecmd

import (
	"context"
	"errors"
	"runtime"
)

var errUnsupportedOS = errors.New("openbee service is not supported on " + runtime.GOOS)

type unsupportedManager struct{}

func NewManager() (Manager, error) { return unsupportedManager{}, nil }

func (unsupportedManager) Install(context.Context, InstallOptions) error { return errUnsupportedOS }
func (unsupportedManager) Uninstall(context.Context) error               { return errUnsupportedOS }
func (unsupportedManager) Start(context.Context) error                   { return errUnsupportedOS }
func (unsupportedManager) Stop(context.Context) error                    { return errUnsupportedOS }
func (unsupportedManager) Status(context.Context) (Status, error)        { return Status{}, errUnsupportedOS }
```

- [ ] **Step 3: Write `command.go` skeleton that wires subcommands**

```go
package servicecmd

import (
	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// NewCommand returns the "service" cobra command group.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: i18n.M.Cmd.Service.Short,
	}
	cmd.AddCommand(
		newInstallCommand(),
		newUninstallCommand(),
		newStartCommand(),
		newStopCommand(),
		newStatusCommand(),
	)
	return cmd
}
```

- [ ] **Step 4: Stub the five subcommand constructors so the package compiles**

Create temporary stubs in `command.go` (will be replaced in later tasks):

```go
func newInstallCommand() *cobra.Command   { return &cobra.Command{Use: "install"} }
func newUninstallCommand() *cobra.Command { return &cobra.Command{Use: "uninstall"} }
func newStartCommand() *cobra.Command     { return &cobra.Command{Use: "start"} }
func newStopCommand() *cobra.Command      { return &cobra.Command{Use: "stop"} }
func newStatusCommand() *cobra.Command    { return &cobra.Command{Use: "status"} }
```

- [ ] **Step 5: Verify it compiles (no i18n keys exist yet — temporarily inline strings)**

Replace `i18n.M.Cmd.Service.Short` with the literal string `"Manage openbee user-level autostart"` for now. The i18n wiring will be added in Task 2.

Run: `go build ./cmd/openbee/...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/
git commit -m "feat(service): scaffold servicecmd package with Manager interface"
```

---

## Task 2: Add i18n keys for service command group

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`
- Modify: `cmd/openbee/internal/cli/servicecmd/command.go`

- [ ] **Step 1: Locate the `Cmd` struct in `messages.go` and add the `Service` field**

Read `internal/infra/i18n/messages.go` to find the existing `Cmd` struct. Add:

```go
// In the Cmd struct:
Service        ServiceCmd        `yaml:"service"`

// New type definitions:
type ServiceCmd struct {
	Short     string `yaml:"short"`
	Install   string `yaml:"install"`
	Uninstall string `yaml:"uninstall"`
	Start     string `yaml:"start"`
	Stop      string `yaml:"stop"`
	StatusS   string `yaml:"status"`
}
```

- [ ] **Step 2: Add `Service` to the `Output` struct**

```go
// In the Output struct:
Service ServiceOutput `yaml:"service"`

type ServiceOutput struct {
	Installed         string `yaml:"installed"`
	Uninstalled       string `yaml:"uninstalled"`
	Started           string `yaml:"started"`
	Stopped           string `yaml:"stopped"`
	AlreadyInstalled  string `yaml:"already_installed"`
	NotInstalled      string `yaml:"not_installed"`
	ConfigMissing     string `yaml:"config_missing"`
	SystemdUnavail    string `yaml:"systemd_unavailable"`
	StatusInstalled   string `yaml:"status_installed"`
	StatusRunState    string `yaml:"status_run_state"`
	StatusPIDUptime   string `yaml:"status_pid_uptime"`
}
```

- [ ] **Step 3: Add flag strings**

In the `Flag` struct, add:

```go
ServiceConfig  string `yaml:"service_config"`
ServiceNoStart string `yaml:"service_no_start"`
ServiceForce   string `yaml:"service_force"`
```

- [ ] **Step 4: Add corresponding entries to `locales/en.yaml`**

```yaml
cmd:
  service:
    short: "Manage openbee user-level autostart"
    install: "Install autostart and (by default) start the service immediately"
    uninstall: "Uninstall autostart"
    start: "Start the service via the OS service manager"
    stop: "Stop the service via the OS service manager"
    status: "Show autostart installation and run state"
flag:
  service_config: "Path to openbee config file (defaults to ~/.openbee/config.yaml)"
  service_no_start: "Only register autostart, do not start the service"
  service_force: "Overwrite existing installation"
output:
  service:
    installed: "service installed at %s"
    uninstalled: "service uninstalled"
    started: "service started"
    stopped: "service stopped"
    already_installed: "service already installed; pass --force to overwrite or run 'openbee service uninstall' first"
    not_installed: "service not installed"
    config_missing: "config file not found at %s, run 'openbee config' first"
    systemd_unavailable: "systemd user session unavailable; see docs/service-autostart.md for prerequisites"
    status_installed: "Installed: %s"
    status_run_state: "State:     %s"
    status_pid_uptime: "PID:       %d (uptime %s)"
```

- [ ] **Step 5: Add the same keys to `locales/zh.yaml` in Chinese**

```yaml
cmd:
  service:
    short: "管理 openbee 用户级开机自启动"
    install: "安装自启动并立即启动服务（默认）"
    uninstall: "卸载自启动"
    start: "通过系统服务管理器启动服务"
    stop: "通过系统服务管理器停止服务"
    status: "查看自启动安装状态与运行状态"
flag:
  service_config: "openbee 配置文件路径（默认 ~/.openbee/config.yaml）"
  service_no_start: "仅注册自启动，不立即启动"
  service_force: "覆盖已有安装"
output:
  service:
    installed: "服务已安装：%s"
    uninstalled: "服务已卸载"
    started: "服务已启动"
    stopped: "服务已停止"
    already_installed: "服务已安装；使用 --force 覆盖或先执行 'openbee service uninstall'"
    not_installed: "服务未安装"
    config_missing: "配置文件不存在：%s，请先运行 'openbee config'"
    systemd_unavailable: "systemd 用户会话不可用；请参考 docs/service-autostart.md"
    status_installed: "已安装：    %s"
    status_run_state: "运行状态：  %s"
    status_pid_uptime: "进程：      PID %d (已运行 %s)"
```

- [ ] **Step 6: Replace the literal string in `command.go` with `i18n.M.Cmd.Service.Short`**

Edit `cmd/openbee/internal/cli/servicecmd/command.go`, restore the `i18n.M.Cmd.Service.Short` reference.

- [ ] **Step 7: Run i18n tests**

Run: `go test ./internal/infra/i18n/...`
Expected: PASS (existing tests verify both YAMLs decode into the same struct shape).

- [ ] **Step 8: Commit**

```bash
git add internal/infra/i18n/ cmd/openbee/internal/cli/servicecmd/command.go
git commit -m "feat(i18n): add service command keys (en/zh)"
```

---

## Task 3: Add resolveInstallOptions helper

**Files:**
- Modify: `cmd/openbee/internal/cli/servicecmd/manager.go`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_test.go`

This helper computes the `InstallOptions` from raw CLI flags by:
1. Resolving the running executable's absolute path.
2. Choosing `--config` if given, else falling back to `~/.openbee/config.yaml`.
3. Verifying the config file exists.
4. Resolving the log path to `~/.openbee/daemon.log`.

- [ ] **Step 1: Write the failing test**

`cmd/openbee/internal/cli/servicecmd/manager_test.go`:

```go
package servicecmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInstallOptions_ExplicitConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := resolveInstallOptions(cfg, false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if opts.ConfigPath != cfg {
		t.Errorf("ConfigPath = %q, want %q", opts.ConfigPath, cfg)
	}
	if opts.AutoStart != true {
		t.Errorf("AutoStart should default to true")
	}
	if opts.ExePath == "" {
		t.Errorf("ExePath empty")
	}
	if opts.LogPath == "" {
		t.Errorf("LogPath empty")
	}
}

func TestResolveInstallOptions_MissingConfig(t *testing.T) {
	_, err := resolveInstallOptions("/nonexistent/path.yaml", false, false)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -run TestResolveInstallOptions`
Expected: FAIL — `resolveInstallOptions` undefined.

- [ ] **Step 3: Implement `resolveInstallOptions` in `manager.go`**

```go
import (
	"errors"
	"fmt"
	"os"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// resolveInstallOptions builds InstallOptions from raw CLI flag values.
// configFlag is the value of --config (empty string means "use default").
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
		cfgPath = home + string(os.PathSeparator) + "config.yaml"
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
```

- [ ] **Step 4: Run test, verify PASS**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -run TestResolveInstallOptions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/manager.go cmd/openbee/internal/cli/servicecmd/manager_test.go
git commit -m "feat(service): add resolveInstallOptions helper"
```

---

## Task 4: Implement install/uninstall/start/stop/status cobra subcommands with fake Manager wiring

**Files:**
- Modify: `cmd/openbee/internal/cli/servicecmd/install.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/uninstall.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/start.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/stop.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/status.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/command.go`
- Create: `cmd/openbee/internal/cli/servicecmd/install_test.go`

The strategy: each subcommand calls a package-level `newManager = NewManager` indirection so tests can substitute a fake.

- [ ] **Step 1: Write the failing test for install with fake Manager**

`install_test.go`:

```go
package servicecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeManager struct {
	installCalls   []InstallOptions
	installErr     error
	uninstallErr   error
	startErr       error
	stopErr        error
	status         Status
	statusErr      error
}

func (f *fakeManager) Install(_ context.Context, opts InstallOptions) error {
	f.installCalls = append(f.installCalls, opts)
	return f.installErr
}
func (f *fakeManager) Uninstall(context.Context) error           { return f.uninstallErr }
func (f *fakeManager) Start(context.Context) error               { return f.startErr }
func (f *fakeManager) Stop(context.Context) error                { return f.stopErr }
func (f *fakeManager) Status(context.Context) (Status, error)    { return f.status, f.statusErr }

func withFake(t *testing.T, fm *fakeManager) {
	t.Helper()
	prev := newManager
	newManager = func() (Manager, error) { return fm, nil }
	t.Cleanup(func() { newManager = prev })
}

func writeFakeConfig(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestInstall_DefaultAutoStart(t *testing.T) {
	fm := &fakeManager{}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(fm.installCalls) != 1 {
		t.Fatalf("Install called %d times", len(fm.installCalls))
	}
	if !fm.installCalls[0].AutoStart {
		t.Errorf("AutoStart should default to true")
	}
	if fm.installCalls[0].Force {
		t.Errorf("Force should default to false")
	}
}

func TestInstall_NoStart(t *testing.T) {
	fm := &fakeManager{}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg, "--no-start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fm.installCalls[0].AutoStart {
		t.Errorf("AutoStart should be false with --no-start")
	}
}

func TestInstall_ManagerError(t *testing.T) {
	fm := &fakeManager{installErr: errors.New("boom")}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"install", "--config", cfg})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/`
Expected: FAIL — `newManager` undefined and subcommands are stubs.

- [ ] **Step 3: Add `newManager` indirection in `manager.go`**

Append to `manager.go`:

```go
// newManager is an indirection point so tests can inject a fake.
var newManager = NewManager
```

- [ ] **Step 4: Implement `install.go`**

```go
package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newInstallCommand() *cobra.Command {
	var (
		configFlag string
		noStart    bool
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: i18n.M.Cmd.Service.Install,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := resolveInstallOptions(configFlag, noStart, force)
			if err != nil {
				return err
			}
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Install(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), i18n.M.Output.Service.Installed+"\n", opts.ConfigPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&configFlag, "config", "", i18n.M.Flag.ServiceConfig)
	cmd.Flags().BoolVar(&noStart, "no-start", false, i18n.M.Flag.ServiceNoStart)
	cmd.Flags().BoolVar(&force, "force", false, i18n.M.Flag.ServiceForce)
	return cmd
}
```

- [ ] **Step 5: Implement `uninstall.go`**

```go
package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: i18n.M.Cmd.Service.Uninstall,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Uninstall(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Uninstalled)
			return nil
		},
	}
}
```

- [ ] **Step 6: Implement `start.go` and `stop.go`**

`start.go`:

```go
package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: i18n.M.Cmd.Service.Start,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Start(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Started)
			return nil
		},
	}
}
```

`stop.go`:

```go
package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: i18n.M.Cmd.Service.Stop,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Stop(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Stopped)
			return nil
		},
	}
}
```

- [ ] **Step 7: Implement `status.go`**

```go
package servicecmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.M.Cmd.Service.StatusS,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			st, err := mgr.Status(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, i18n.M.Output.Service.StatusInstalled+"\n", boolYesNo(st.Installed))
			fmt.Fprintf(out, i18n.M.Output.Service.StatusRunState+"\n", st.RunState.String())
			if st.RunState == RunStateRunning && st.PID > 0 {
				fmt.Fprintf(out, i18n.M.Output.Service.StatusPIDUptime+"\n", st.PID, formatUptime(time.Duration(st.UptimeSecs)*time.Second))
			}
			return nil
		},
	}
}

func boolYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func formatUptime(d time.Duration) string {
	secs := int64(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60)
}
```

- [ ] **Step 8: Run tests, verify PASS**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -v`
Expected: PASS for `TestInstall_*`, `TestResolveInstallOptions_*`.

- [ ] **Step 9: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/
git commit -m "feat(service): implement subcommand handlers with Manager indirection"
```

---

## Task 5: Wire `servicecmd` into root.go

**Files:**
- Modify: `cmd/openbee/internal/cli/root.go`

- [ ] **Step 1: Add import and `root.AddCommand` call**

Edit `cmd/openbee/internal/cli/root.go`:

```go
import (
	// existing imports...
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/servicecmd"
)

// in NewRoot, after the existing AddCommand calls:
root.AddCommand(servicecmd.NewCommand())
```

- [ ] **Step 2: Run build**

Run: `go build ./cmd/openbee/...`
Expected: success.

- [ ] **Step 3: Run all root tests**

Run: `go test ./cmd/openbee/...`
Expected: PASS.

- [ ] **Step 4: Manual smoke test**

Run: `go run ./cmd/openbee service --help`
Expected: shows `Manage openbee user-level autostart` and lists 5 subcommands.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/internal/cli/root.go
git commit -m "feat(cli): register service command in root"
```

---

## Task 6: macOS — embed plist template and add render test

**Files:**
- Create: `cmd/openbee/internal/cli/servicecmd/templates/launchd.plist.tmpl`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_darwin.go`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_darwin_test.go`

- [ ] **Step 1: Write template file `launchd.plist.tmpl`**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.theopenbee.openbee</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{.ExePath}}</string>
    <string>server</string>
    <string>-c</string>
    <string>{{.ConfigPath}}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>StandardOutPath</key>
  <string>{{.LogPath}}</string>
  <key>StandardErrorPath</key>
  <string>{{.LogPath}}</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>{{.Home}}</string>
    <key>PATH</key>
    <string>{{.Path}}</string>
  </dict>
</dict>
</plist>
```

- [ ] **Step 2: Write the failing test for render**

`manager_darwin_test.go`:

```go
//go:build darwin

package servicecmd

import (
	"strings"
	"testing"
)

func TestRenderLaunchdPlist(t *testing.T) {
	got, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/Users/me/.openbee/config.yaml",
		LogPath:    "/Users/me/.openbee/daemon.log",
		Home:       "/Users/me",
		Path:       "/usr/local/bin:/usr/bin:/bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>com.theopenbee.openbee</string>",
		"<string>/usr/local/bin/openbee</string>",
		"<string>server</string>",
		"<string>-c</string>",
		"<string>/Users/me/.openbee/config.yaml</string>",
		"<key>KeepAlive</key>",
		"<integer>10</integer>",
		"<string>/Users/me/.openbee/daemon.log</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered plist missing %q\nfull:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 3: Run test, verify it fails**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderLaunchdPlist`
Expected: FAIL — `renderLaunchdPlist` undefined.

- [ ] **Step 4: Implement renderer in `manager_darwin.go`**

```go
//go:build darwin

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/launchd.plist.tmpl
var darwinTemplatesFS embed.FS

const (
	launchdLabel = "com.theopenbee.openbee"
)

type launchdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
	Path       string
}

func renderLaunchdPlist(d launchdTemplateData) (string, error) {
	tmplBytes, err := darwinTemplatesFS.ReadFile("templates/launchd.plist.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("launchd").Parse(string(tmplBytes))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type darwinManager struct{}

func NewManager() (Manager, error) { return darwinManager{}, nil }

func (darwinManager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

// The remaining Install/Uninstall/Start/Stop/Status methods are added in Task 7.
func (darwinManager) Install(context.Context, InstallOptions) error { return errors.New("not implemented") }
func (darwinManager) Uninstall(context.Context) error               { return errors.New("not implemented") }
func (darwinManager) Start(context.Context) error                   { return errors.New("not implemented") }
func (darwinManager) Stop(context.Context) error                    { return errors.New("not implemented") }
func (darwinManager) Status(context.Context) (Status, error)        { return Status{}, errors.New("not implemented") }

// guiTarget builds the launchd domain target for the current user.
func guiTarget() string {
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return "gui/0"
	}
	return "gui/" + u.Uid
}

// Silence unused import warnings until Task 7 fills in real methods.
var _ = []any{i18n.M, time.Now, exec.Command, strconv.Itoa, strings.TrimSpace, fmt.Sprintf}
```

- [ ] **Step 5: Run test, verify PASS**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderLaunchdPlist`
Expected: PASS (on macOS only — other OSes skip via build tag).

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/templates/launchd.plist.tmpl cmd/openbee/internal/cli/servicecmd/manager_darwin.go cmd/openbee/internal/cli/servicecmd/manager_darwin_test.go
git commit -m "feat(service): add launchd plist template and renderer"
```

---

## Task 7: macOS — implement Install/Uninstall/Start/Stop/Status

**Files:**
- Modify: `cmd/openbee/internal/cli/servicecmd/manager_darwin.go`
- Modify: `cmd/openbee/internal/cli/servicecmd/manager_darwin_test.go`

- [ ] **Step 1: Write the failing test for Install path-write behavior**

Add to `manager_darwin_test.go`:

```go
func TestDarwinInstall_WritesPlist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	mgr := darwinManager{}
	// Override exec so we don't actually call launchctl.
	prev := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/launchctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }
	t.Cleanup(func() { execLookPath = prev; runCommand = prevRun })

	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)
	log := filepath.Join(tmp, "daemon.log")

	if err := mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    log,
		AutoStart:  false,
	}); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(tmp, "Library", "LaunchAgents", "com.theopenbee.openbee.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(data), cfg) {
		t.Errorf("plist missing config path")
	}
}
```

The test imports `"context"`, `"os"`, `"path/filepath"`, `"strings"`.

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — `Install` returns "not implemented".

- [ ] **Step 3: Replace stub methods with real implementation in `manager_darwin.go`**

```go
//go:build darwin

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/launchd.plist.tmpl
var darwinTemplatesFS embed.FS

const launchdLabel = "com.theopenbee.openbee"

// Indirections for tests.
var (
	execLookPath = exec.LookPath
	runCommand   = defaultRunCommand
)

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type launchdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
	Path       string
}

func renderLaunchdPlist(d launchdTemplateData) (string, error) {
	b, err := darwinTemplatesFS.ReadFile("templates/launchd.plist.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("launchd").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type darwinManager struct{}

func NewManager() (Manager, error) { return darwinManager{}, nil }

func (darwinManager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func (m darwinManager) Install(ctx context.Context, opts InstallOptions) error {
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(pp); err == nil && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	if err := os.MkdirAll(filepath.Dir(pp), 0o755); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	plist, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		Home:       home,
		Path:       os.Getenv("PATH"),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(pp, []byte(plist), 0o644); err != nil {
		return err
	}
	if _, err := execLookPath("launchctl"); err != nil {
		return fmt.Errorf("launchctl not found: %w", err)
	}
	if out, err := runCommand(ctx, "launchctl", "bootstrap", guiTarget(), pp); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !opts.AutoStart {
		return nil
	}
	if out, err := runCommand(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m darwinManager) Uninstall(ctx context.Context) error {
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	_, _ = runCommand(ctx, "launchctl", "bootout", guiTarget()+"/"+launchdLabel)
	if err := os.Remove(pp); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (darwinManager) Start(ctx context.Context) error {
	if out, err := runCommand(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (darwinManager) Stop(ctx context.Context) error {
	if out, err := runCommand(ctx, "launchctl", "kill", "SIGTERM", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kill: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	launchdStateRe = regexp.MustCompile(`state\s*=\s*(\w+)`)
	launchdPIDRe   = regexp.MustCompile(`pid\s*=\s*(\d+)`)
)

func (m darwinManager) Status(ctx context.Context) (Status, error) {
	pp, err := m.plistPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if _, err := os.Stat(pp); err == nil {
		st.Installed = true
	}
	out, err := runCommand(ctx, "launchctl", "print", guiTarget()+"/"+launchdLabel)
	if err != nil {
		if !st.Installed {
			return st, nil
		}
		st.RunState = RunStateStopped
		return st, nil
	}
	text := string(out)
	if m := launchdStateRe.FindStringSubmatch(text); len(m) == 2 {
		if strings.Contains(strings.ToLower(m[1]), "running") {
			st.RunState = RunStateRunning
		} else {
			st.RunState = RunStateStopped
		}
	}
	if m := launchdPIDRe.FindStringSubmatch(text); len(m) == 2 {
		if pid, err := strconv.Atoi(m[1]); err == nil {
			st.PID = pid
			st.UptimeSecs = readUptime(pid)
		}
	}
	return st, nil
}

// readUptime returns the process uptime in seconds by parsing `ps -o etimes= -p PID`.
func readUptime(pid int) int64 {
	out, err := exec.Command("ps", "-o", "etimes=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
		return v
	}
	return 0
}

func guiTarget() string {
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return "gui/0"
	}
	return "gui/" + u.Uid
}

var _ = time.Now // reserved for future uptime fallback
```

- [ ] **Step 4: Run tests, verify PASS**

Run: `go test ./cmd/openbee/internal/cli/servicecmd/ -v` (on macOS)
Expected: PASS for all darwin tests.

- [ ] **Step 5: Build all platforms via cross-compile sanity check**

Run:
```bash
GOOS=linux go build ./cmd/openbee/...
GOOS=windows go build ./cmd/openbee/...
GOOS=darwin go build ./cmd/openbee/...
```
Expected: all succeed (Linux/Windows still use stub `manager_other.go` until Task 8/10).

Note: After Task 8 adds `manager_linux.go`, the Linux cross-build will exercise that file instead.

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/manager_darwin.go cmd/openbee/internal/cli/servicecmd/manager_darwin_test.go
git commit -m "feat(service): implement darwin Manager using launchctl"
```

---

## Task 8: Linux — embed systemd unit template and add render test

**Files:**
- Create: `cmd/openbee/internal/cli/servicecmd/templates/systemd.service.tmpl`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_linux.go`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_linux_test.go`

- [ ] **Step 1: Write template `systemd.service.tmpl`**

```ini
[Unit]
Description=OpenBee Worker Daemon
After=default.target

[Service]
Type=simple
ExecStart={{.ExePath}} server -c {{.ConfigPath}}
Restart=on-failure
RestartSec=10
StandardOutput=append:{{.LogPath}}
StandardError=append:{{.LogPath}}
Environment=HOME={{.Home}}

[Install]
WantedBy=default.target
```

- [ ] **Step 2: Write failing render test in `manager_linux_test.go`**

```go
//go:build linux

package servicecmd

import (
	"strings"
	"testing"
)

func TestRenderSystemdUnit(t *testing.T) {
	got, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/home/me/.openbee/config.yaml",
		LogPath:    "/home/me/.openbee/daemon.log",
		Home:       "/home/me",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ExecStart=/usr/local/bin/openbee server -c /home/me/.openbee/config.yaml",
		"Restart=on-failure",
		"RestartSec=10",
		"StandardOutput=append:/home/me/.openbee/daemon.log",
		"WantedBy=default.target",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit missing %q\nfull:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 3: Run test, verify it fails (on Linux)**

Run: `GOOS=linux go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderSystemdUnit`
Expected: FAIL — `renderSystemdUnit` undefined.

- [ ] **Step 4: Implement `manager_linux.go` (template + Manager stub)**

```go
//go:build linux

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"text/template"
)

//go:embed templates/systemd.service.tmpl
var linuxTemplatesFS embed.FS

const systemdUnitName = "openbee.service"

type systemdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
}

func renderSystemdUnit(d systemdTemplateData) (string, error) {
	b, err := linuxTemplatesFS.ReadFile("templates/systemd.service.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("systemd").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type linuxManager struct{}

func NewManager() (Manager, error) { return linuxManager{}, nil }

// Stubs filled in by Task 9.
func (linuxManager) Install(context.Context, InstallOptions) error { return errors.New("not implemented") }
func (linuxManager) Uninstall(context.Context) error               { return errors.New("not implemented") }
func (linuxManager) Start(context.Context) error                   { return errors.New("not implemented") }
func (linuxManager) Stop(context.Context) error                    { return errors.New("not implemented") }
func (linuxManager) Status(context.Context) (Status, error)        { return Status{}, errors.New("not implemented") }
```

- [ ] **Step 5: Run test, verify PASS**

Run: `GOOS=linux go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderSystemdUnit`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/templates/systemd.service.tmpl cmd/openbee/internal/cli/servicecmd/manager_linux.go cmd/openbee/internal/cli/servicecmd/manager_linux_test.go
git commit -m "feat(service): add systemd unit template and renderer"
```

---

## Task 9: Linux — implement Install/Uninstall/Start/Stop/Status via systemctl --user

**Files:**
- Modify: `cmd/openbee/internal/cli/servicecmd/manager_linux.go`

- [ ] **Step 1: Replace the stub `linuxManager` with the real implementation**

```go
//go:build linux

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/systemd.service.tmpl
var linuxTemplatesFS embed.FS

const systemdUnitName = "openbee.service"

var (
	execLookPath = exec.LookPath
	runCommand   = defaultRunCommand
)

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type systemdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
}

func renderSystemdUnit(d systemdTemplateData) (string, error) {
	b, err := linuxTemplatesFS.ReadFile("templates/systemd.service.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("systemd").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type linuxManager struct{}

func NewManager() (Manager, error) {
	if _, err := execLookPath("systemctl"); err != nil {
		return nil, errors.New(i18n.M.Output.Service.SystemdUnavail)
	}
	return linuxManager{}, nil
}

func (linuxManager) unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdUnitName), nil
}

func (m linuxManager) Install(ctx context.Context, opts InstallOptions) error {
	up, err := m.unitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(up); err == nil && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	if err := os.MkdirAll(filepath.Dir(up), 0o755); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	unit, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		Home:       home,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(up, []byte(unit), 0o644); err != nil {
		return err
	}
	if out, err := runCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	enableArgs := []string{"--user", "enable", systemdUnitName}
	if opts.AutoStart {
		enableArgs = []string{"--user", "enable", "--now", systemdUnitName}
	}
	if out, err := runCommand(ctx, "systemctl", enableArgs...); err != nil {
		return fmt.Errorf("systemctl enable: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m linuxManager) Uninstall(ctx context.Context) error {
	up, err := m.unitPath()
	if err != nil {
		return err
	}
	_, _ = runCommand(ctx, "systemctl", "--user", "disable", "--now", systemdUnitName)
	if err := os.Remove(up); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = runCommand(ctx, "systemctl", "--user", "daemon-reload")
	return nil
}

func (linuxManager) Start(ctx context.Context) error {
	if out, err := runCommand(ctx, "systemctl", "--user", "start", systemdUnitName); err != nil {
		return fmt.Errorf("systemctl start: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (linuxManager) Stop(ctx context.Context) error {
	if out, err := runCommand(ctx, "systemctl", "--user", "stop", systemdUnitName); err != nil {
		return fmt.Errorf("systemctl stop: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m linuxManager) Status(ctx context.Context) (Status, error) {
	up, err := m.unitPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if _, err := os.Stat(up); err == nil {
		st.Installed = true
	}
	out, err := runCommand(ctx, "systemctl", "--user", "show", "-p", "ActiveState,SubState,MainPID,ExecMainStartTimestamp", systemdUnitName)
	if err != nil {
		if !st.Installed {
			return st, nil
		}
		st.RunState = RunStateStopped
		return st, nil
	}
	props := parseSystemctlShow(string(out))
	switch props["ActiveState"] {
	case "active":
		st.RunState = RunStateRunning
	case "failed":
		st.RunState = RunStateFailed
	default:
		st.RunState = RunStateStopped
	}
	if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
		st.PID = pid
		st.UptimeSecs = readUptime(pid)
	}
	return st, nil
}

func parseSystemctlShow(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "="); i > 0 {
			out[line[:i]] = line[i+1:]
		}
	}
	return out
}

func readUptime(pid int) int64 {
	out, err := exec.Command("ps", "-o", "etimes=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
		return v
	}
	return 0
}
```

- [ ] **Step 2: Write a fake-exec test for Install path-write behavior**

Append to `manager_linux_test.go`:

```go
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxInstall_WritesUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	if err := mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
		AutoStart:  false,
	}); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(tmp, ".config", "systemd", "user", "openbee.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(data), cfg) {
		t.Errorf("unit missing config path")
	}
}
```

- [ ] **Step 3: Run tests (on Linux), verify PASS**

Run: `GOOS=linux go test ./cmd/openbee/internal/cli/servicecmd/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/manager_linux.go cmd/openbee/internal/cli/servicecmd/manager_linux_test.go
git commit -m "feat(service): implement linux Manager using systemctl --user"
```

---

## Task 10: Windows — embed Task Scheduler XML template + render test

**Files:**
- Create: `cmd/openbee/internal/cli/servicecmd/templates/schtask.xml.tmpl`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_windows.go`
- Create: `cmd/openbee/internal/cli/servicecmd/manager_windows_test.go`

- [ ] **Step 1: Write template `schtask.xml.tmpl`**

```xml
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>OpenBee user-level autostart</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>{{.UserId}}</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>{{.UserId}}</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <Hidden>true</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>cmd.exe</Command>
      <Arguments>/c &quot;&quot;{{.ExePath}}&quot; server -c &quot;{{.ConfigPath}}&quot; &gt;&gt; &quot;{{.LogPath}}&quot; 2&gt;&amp;1&quot;</Arguments>
    </Exec>
  </Actions>
</Task>
```

- [ ] **Step 2: Write failing render test**

`manager_windows_test.go`:

```go
//go:build windows

package servicecmd

import (
	"strings"
	"testing"
)

func TestRenderSchtaskXML(t *testing.T) {
	got, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     "DESKTOP-A\\me",
		ExePath:    `C:\Program Files\openbee\openbee.exe`,
		ConfigPath: `C:\Users\me\.openbee\config.yaml`,
		LogPath:    `C:\Users\me\.openbee\daemon.log`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<UserId>DESKTOP-A\\me</UserId>",
		"<RunLevel>LeastPrivilege</RunLevel>",
		"<Hidden>true</Hidden>",
		"<Interval>PT1M</Interval>",
		"openbee.exe",
		"server -c",
		"daemon.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("XML missing %q\nfull:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 3: Run test (on Windows), verify FAIL**

Run: `GOOS=windows go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderSchtaskXML`
Expected: FAIL — undefined.

- [ ] **Step 4: Implement `manager_windows.go`**

```go
//go:build windows

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"text/template"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

type schtaskTemplateData struct {
	UserId     string
	ExePath    string
	ConfigPath string
	LogPath    string
}

func renderSchtaskXML(d schtaskTemplateData) (string, error) {
	b, err := windowsTemplatesFS.ReadFile("templates/schtask.xml.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("schtask").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type windowsManager struct{}

func NewManager() (Manager, error) { return windowsManager{}, nil }

// Stubs filled in by Task 11.
func (windowsManager) Install(context.Context, InstallOptions) error { return errors.New("not implemented") }
func (windowsManager) Uninstall(context.Context) error               { return errors.New("not implemented") }
func (windowsManager) Start(context.Context) error                   { return errors.New("not implemented") }
func (windowsManager) Stop(context.Context) error                    { return errors.New("not implemented") }
func (windowsManager) Status(context.Context) (Status, error)        { return Status{}, errors.New("not implemented") }
```

- [ ] **Step 5: Run test, verify PASS**

Run: `GOOS=windows go test ./cmd/openbee/internal/cli/servicecmd/ -run TestRenderSchtaskXML`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/templates/schtask.xml.tmpl cmd/openbee/internal/cli/servicecmd/manager_windows.go cmd/openbee/internal/cli/servicecmd/manager_windows_test.go
git commit -m "feat(service): add Task Scheduler XML template and renderer"
```

---

## Task 11: Windows — implement Install/Uninstall/Start/Stop/Status via schtasks

**Files:**
- Modify: `cmd/openbee/internal/cli/servicecmd/manager_windows.go`

- [ ] **Step 1: Replace stub `windowsManager` with real implementation**

```go
//go:build windows

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"text/template"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

var (
	execLookPath = exec.LookPath
	runCommand   = defaultRunCommand
)

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type schtaskTemplateData struct {
	UserId     string
	ExePath    string
	ConfigPath string
	LogPath    string
}

func renderSchtaskXML(d schtaskTemplateData) (string, error) {
	b, err := windowsTemplatesFS.ReadFile("templates/schtask.xml.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("schtask").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type windowsManager struct{}

func NewManager() (Manager, error) { return windowsManager{}, nil }

func currentUserID() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func (windowsManager) taskExists(ctx context.Context) bool {
	_, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName)
	return err == nil
}

func (m windowsManager) Install(ctx context.Context, opts InstallOptions) error {
	if m.taskExists(ctx) && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	uid, err := currentUserID()
	if err != nil {
		return err
	}
	xml, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     uid,
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "openbee-schtask-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	// Task Scheduler requires UTF-16LE with BOM for /XML import.
	if _, err := tmp.Write(encodeUTF16LE(xml)); err != nil {
		return err
	}
	tmp.Close()

	args := []string{"/Create", "/XML", tmp.Name(), "/TN", schtaskName, "/F"}
	if out, err := runCommand(ctx, "schtasks", args...); err != nil {
		return fmt.Errorf("schtasks /Create: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !opts.AutoStart {
		return nil
	}
	if out, err := runCommand(ctx, "schtasks", "/Run", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /Run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Uninstall(ctx context.Context) error {
	_, _ = runCommand(ctx, "schtasks", "/End", "/TN", schtaskName)
	if out, err := runCommand(ctx, "schtasks", "/Delete", "/TN", schtaskName, "/F"); err != nil {
		// If task doesn't exist, treat as success.
		if strings.Contains(strings.ToLower(string(out)), "cannot find") {
			return nil
		}
		return fmt.Errorf("schtasks /Delete: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Start(ctx context.Context) error {
	if out, err := runCommand(ctx, "schtasks", "/Run", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /Run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Stop(ctx context.Context) error {
	if out, err := runCommand(ctx, "schtasks", "/End", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /End: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m windowsManager) Status(ctx context.Context) (Status, error) {
	st := Status{}
	if !m.taskExists(ctx) {
		return st, nil
	}
	st.Installed = true
	out, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName, "/V", "/FO", "LIST")
	if err != nil {
		st.RunState = RunStateUnknown
		return st, nil
	}
	status := parseSchtasksField(string(out), "Status:")
	switch strings.TrimSpace(status) {
	case "Running":
		st.RunState = RunStateRunning
	case "Ready":
		st.RunState = RunStateStopped
	default:
		st.RunState = RunStateUnknown
	}
	if st.RunState == RunStateRunning {
		if pid := lookupOpenbeePID(ctx); pid > 0 {
			st.PID = pid
			st.UptimeSecs = 0 // uptime via tasklist is unreliable; skipped per spec
		}
	}
	return st, nil
}

func parseSchtasksField(s, key string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, key); i >= 0 {
			return strings.TrimSpace(line[i+len(key):])
		}
	}
	return ""
}

func lookupOpenbeePID(ctx context.Context) int {
	out, err := runCommand(ctx, "tasklist", "/FI", "IMAGENAME eq openbee.exe", "/FO", "CSV", "/NH")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			pidStr := strings.Trim(fields[1], `" `)
			if pid, err := strconv.Atoi(pidStr); err == nil {
				return pid
			}
		}
	}
	return 0
}

// encodeUTF16LE encodes a string as UTF-16LE with a BOM.
func encodeUTF16LE(s string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	for _, r := range s {
		if r > 0xFFFF {
			// Surrogate pair (rare for our XML, but safe to handle).
			r -= 0x10000
			hi := uint16(0xD800 + (r >> 10))
			lo := uint16(0xDC00 + (r & 0x3FF))
			buf.WriteByte(byte(hi))
			buf.WriteByte(byte(hi >> 8))
			buf.WriteByte(byte(lo))
			buf.WriteByte(byte(lo >> 8))
		} else {
			buf.WriteByte(byte(r))
			buf.WriteByte(byte(r >> 8))
		}
	}
	return buf.Bytes()
}
```

- [ ] **Step 2: Add UTF-16 encoding unit test**

Append to `manager_windows_test.go`:

```go
func TestEncodeUTF16LE_HasBOM(t *testing.T) {
	got := encodeUTF16LE("A")
	if len(got) != 4 {
		t.Fatalf("want 4 bytes, got %d", len(got))
	}
	if got[0] != 0xFF || got[1] != 0xFE {
		t.Errorf("missing BOM: %x %x", got[0], got[1])
	}
	if got[2] != 'A' || got[3] != 0 {
		t.Errorf("wrong encoding: %x %x", got[2], got[3])
	}
}
```

- [ ] **Step 3: Run tests, verify PASS**

Run: `GOOS=windows go test ./cmd/openbee/internal/cli/servicecmd/ -v` (cross-compile on macOS via test workflow, or run on actual Windows runner)
Expected: PASS for render and encoding tests.

- [ ] **Step 4: Cross-platform build sanity check**

Run:
```bash
GOOS=darwin go build ./cmd/openbee/...
GOOS=linux go build ./cmd/openbee/...
GOOS=windows go build ./cmd/openbee/...
```
Expected: all succeed.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/internal/cli/servicecmd/manager_windows.go cmd/openbee/internal/cli/servicecmd/manager_windows_test.go
git commit -m "feat(service): implement windows Manager using schtasks"
```

---

## Task 12: User documentation and README pointer

**Files:**
- Create: `docs/service-autostart.md`
- Modify: `README.md`
- Modify: `README.zh.md`

- [ ] **Step 1: Write `docs/service-autostart.md`**

```markdown
# Autostart (`openbee service`)

The `openbee service` command group registers `openbee server` to start automatically when the current user logs in. All three operating systems use **user-level** mechanisms; no sudo or administrator rights are required.

## Quick start

```bash
openbee service install    # register and start
openbee service status
openbee service stop
openbee service start
openbee service uninstall  # remove registration
```

Pass `--config <path>` on `install` to point at a non-default config file. Pass `--no-start` to register without starting immediately. Pass `--force` to overwrite an existing registration.

## Platform details

### macOS

A LaunchAgent plist is written to `~/Library/LaunchAgents/com.theopenbee.openbee.plist` and registered via `launchctl bootstrap`. The process is auto-restarted on crash (`KeepAlive=true`, `ThrottleInterval=10`).

### Linux

A systemd user unit is written to `~/.config/systemd/user/openbee.service` and enabled via `systemctl --user enable --now`. The process is auto-restarted on failure (`Restart=on-failure`, `RestartSec=10`).

**Prerequisites:**
- A working systemd user session. Distributions without systemd (Alpine, certain WSL setups, minimal containers) are unsupported — use `openbee server` directly or write your own init script.
- If you want the service to survive logout, enable **user lingering**: `sudo loginctl enable-linger $USER`.

### Windows

A Scheduled Task named `OpenBee` is created with a logon trigger for the current user. Failure retry is set to 3 attempts spaced 1 minute apart. The task runs hidden — open `taskschd.msc` and navigate to **Task Scheduler Library > OpenBee** to inspect.

## Logs

All platforms redirect stdout/stderr to `~/.openbee/daemon.log` (same file as `openbee server`).

## Coexisting with `openbee server`

Once `service install` completes, the service manager owns the lifecycle. Running `openbee server` manually will refuse to start because the PID file is already taken by the auto-started instance. Use `openbee service stop` first.
```

- [ ] **Step 2: Add a brief autostart section to `README.md`**

Find the "Quick Start" area and add after the install steps:

```markdown
### Step 3 (optional): Enable autostart at login

```bash
openbee service install
```

This registers `openbee server` to start automatically when you log in on macOS, Linux, or Windows. See [docs/service-autostart.md](docs/service-autostart.md) for details.
```

- [ ] **Step 3: Mirror the section in `README.zh.md`**

```markdown
### 第 3 步（可选）：开机自启动

```bash
openbee service install
```

此命令会把 `openbee server` 注册到 macOS / Linux / Windows 的用户登录自启动。详见 [docs/service-autostart.md](docs/service-autostart.md)。
```

- [ ] **Step 4: Commit**

```bash
git add docs/service-autostart.md README.md README.zh.md
git commit -m "docs(service): document openbee service subcommand"
```

---

## Task 13: Changelog entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add Unreleased entry**

Read the top of `CHANGELOG.md` to follow existing format. Add under the Unreleased section:

```markdown
### Added
- `openbee service install/uninstall/start/stop/status` subcommand group for one-click user-level autostart on macOS (launchd), Linux (systemd --user), and Windows (Task Scheduler). See `docs/service-autostart.md`.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): add openbee service autostart entry"
```

---

## Task 14: End-to-end manual smoke test on the host OS

**Files:**
- None (manual verification)

- [ ] **Step 1: Build the binary**

Run: `go build -o /tmp/openbee ./cmd/openbee`
Expected: success.

- [ ] **Step 2: Generate a minimal config if not present**

Run: `ls ~/.openbee/config.yaml || /tmp/openbee config`
Expected: file exists afterward.

- [ ] **Step 3: Install autostart**

Run: `/tmp/openbee service install`
Expected: prints "service installed at <config path>"; process visible in:
- macOS: `launchctl list | grep com.theopenbee.openbee`
- Linux: `systemctl --user status openbee.service`
- Windows: `schtasks /Query /TN OpenBee /V`

- [ ] **Step 4: Status**

Run: `/tmp/openbee service status`
Expected: `Installed: yes`, `State: running`, PID and uptime shown.

- [ ] **Step 5: Stop and re-start via service**

Run: `/tmp/openbee service stop && /tmp/openbee service start`
Expected: process restarts, status returns to running.

- [ ] **Step 6: Uninstall and verify cleanup**

Run: `/tmp/openbee service uninstall && /tmp/openbee service status`
Expected: `Installed: no`. Platform artifact (plist / unit / task) is gone.

- [ ] **Step 7: No commit needed**

Manual smoke test only — no code change.

---

## Self-Review Notes

After writing this plan I checked against the spec:

- ✅ Section 1–2 (background/decisions): captured in plan header.
- ✅ Section 3 (CLI surface): Tasks 4 and 5.
- ✅ Section 4 (code structure): file structure list + Task 1.
- ✅ Section 5 (Manager interface): Task 1.
- ✅ Section 6.1 (macOS): Tasks 6, 7.
- ✅ Section 6.2 (Linux): Tasks 8, 9.
- ✅ Section 6.3 (Windows): Tasks 10, 11.
- ✅ Section 7 (logs/PID/concurrency): config path & log path handled in resolveInstallOptions (Task 3); reuses `config.DaemonLogFile()`.
- ✅ Section 8 (errors/i18n): Task 2 keys + use sites in Tasks 4, 7, 9, 11.
- ✅ Section 9 (testing): TDD steps throughout; render snapshot tests in Tasks 6, 8, 10; fake-exec integration in 7, 9.
- ✅ Section 10 (docs): Task 12.
- ✅ Section 11 (phased delivery): the task order mirrors the spec's phase order (interface → macOS → Linux → Windows → docs).

Type consistency: `Manager` / `InstallOptions` / `Status` / `RunState` names used identically in all tasks. `execLookPath` and `runCommand` indirections share the same signatures across darwin/linux/windows.

No placeholders, no TBDs. Each code step has the complete code an engineer can paste in.
