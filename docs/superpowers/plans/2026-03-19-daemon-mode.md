# Daemon Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbee server --daemon` background mode with companion `stop`, `restart`, and `status` commands, cross-platform on macOS, Linux, and Windows.

**Architecture:** Re-exec model — parent detects `--daemon` flag, re-launches itself with `OPENBEE_DAEMON=1` env var using platform-specific detach attributes, polls child liveness for 2 s, writes PID file, then exits. Child redirects fd1+fd2 to `~/.openbee/openbee.log` **before** `logger.Init`, then starts the server normally. Platform-specific syscall code is isolated behind `//go:build` tags.

**Tech Stack:** Go, cobra, `syscall` package (Unix + Windows), `golang.org/x/sys/windows` (Windows constants only)

---

## File Map

| File | Role |
|------|------|
| `cmd/openbee/daemon.go` | Shared: file paths, PID file R/W, `isDaemonChild()`, `daemonize()`, `formatUptime()` |
| `cmd/openbee/daemon_unix.go` | `//go:build !windows` — `spawnDaemon()`, `isAlive()`, `stopProcess()`, `redirectStdio()` |
| `cmd/openbee/daemon_windows.go` | `//go:build windows` — same interface, Windows syscalls |
| `cmd/openbee/daemon_test.go` | Unit tests for PID file helpers, `isDaemonChild`, `isAlive`, `formatUptime` |
| `cmd/openbee/server.go` | Add `--daemon` / `-d` flag; dispatch to `daemonize()` or `redirectStdio()` in `RunE` |
| `cmd/openbee/stop.go` | `stopCmd` — read PID, call `stopProcess()`, remove PID file |
| `cmd/openbee/restart.go` | `restartCmd` — stop + `daemonize()` |
| `cmd/openbee/status.go` | `statusCmd` — read PID, `isAlive()`, print status + uptime |

---

### Task 1: Shared daemon helpers (`daemon.go`)

**Files:**
- Create: `cmd/openbee/daemon.go`
- Create: `cmd/openbee/daemon_test.go`

- [ ] **Step 1: Write failing tests for PID file helpers**

  Create `cmd/openbee/daemon_test.go`:

  ```go
  package main

  import (
  	"os"
  	"path/filepath"
  	"testing"
  	"time"

  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  )

  func TestWriteReadPIDFile(t *testing.T) {
  	dir := t.TempDir()
  	path := filepath.Join(dir, "test.pid")

  	ts := time.Now().Unix()
  	require.NoError(t, writePIDFileTo(path, 12345, ts))

  	pid, got, err := readPIDFileFrom(path)
  	require.NoError(t, err)
  	assert.Equal(t, 12345, pid)
  	assert.Equal(t, ts, got)
  }

  func TestReadPIDFileMissing(t *testing.T) {
  	_, _, err := readPIDFileFrom("/nonexistent/path/openbee.pid")
  	assert.Error(t, err)
  }

  func TestIsDaemonChild(t *testing.T) {
  	os.Unsetenv("OPENBEE_DAEMON")
  	assert.False(t, isDaemonChild())

  	t.Setenv("OPENBEE_DAEMON", "1")
  	assert.True(t, isDaemonChild())
  }

  func TestFormatUptime(t *testing.T) {
  	assert.Equal(t, "0s", formatUptime(0))
  	assert.Equal(t, "45s", formatUptime(45))
  	assert.Equal(t, "5m 3s", formatUptime(303))
  	assert.Equal(t, "2h 5m", formatUptime(7530))
  	assert.Equal(t, "25h 0m", formatUptime(90000))
  }
  ```

- [ ] **Step 2: Run tests to confirm they fail**

  ```bash
  cd /Users/tengyongzhi/work/bot-workspaces/openbee
  go test ./cmd/openbee/ -run "TestWriteReadPIDFile|TestReadPIDFileMissing|TestIsDaemonChild|TestFormatUptime" -v
  ```

  Expected: compile error — functions not defined yet.

- [ ] **Step 3: Create `cmd/openbee/daemon.go`**

  ```go
  package main

  import (
  	"fmt"
  	"os"
  	"path/filepath"
  	"strconv"
  	"strings"
  	"time"
  )

  // daemonEnvKey is the env var set on the daemon child to distinguish it from the parent.
  const daemonEnvKey = "OPENBEE_DAEMON"

  // daemonPIDFile returns the path to the PID file.
  func daemonPIDFile() string {
  	home, _ := os.UserHomeDir()
  	return filepath.Join(home, ".openbee", "openbee.pid")
  }

  // daemonLogFile returns the path to the daemon log file.
  func daemonLogFile() string {
  	home, _ := os.UserHomeDir()
  	return filepath.Join(home, ".openbee", "openbee.log")
  }

  // isDaemonChild reports whether this process was launched as a daemon child.
  func isDaemonChild() bool {
  	return os.Getenv(daemonEnvKey) == "1"
  }

  // writePIDFile writes pid and start timestamp to daemonPIDFile().
  func writePIDFile(pid int, startTS int64) error {
  	return writePIDFileTo(daemonPIDFile(), pid, startTS)
  }

  // writePIDFileTo writes pid and start timestamp to the given path.
  func writePIDFileTo(path string, pid int, startTS int64) error {
  	content := fmt.Sprintf("%d\n%d\n", pid, startTS)
  	return os.WriteFile(path, []byte(content), 0644)
  }

  // readPIDFile reads pid and start timestamp from daemonPIDFile().
  func readPIDFile() (pid int, startTS int64, err error) {
  	return readPIDFileFrom(daemonPIDFile())
  }

  // readPIDFileFrom reads pid and start timestamp from the given path.
  func readPIDFileFrom(path string) (pid int, startTS int64, err error) {
  	data, err := os.ReadFile(path)
  	if err != nil {
  		return 0, 0, err
  	}
  	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
  	if len(lines) < 2 {
  		return 0, 0, fmt.Errorf("malformed pid file")
  	}
  	pid64, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
  	if err != nil {
  		return 0, 0, fmt.Errorf("parse pid: %w", err)
  	}
  	ts, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
  	if err != nil {
  		return 0, 0, fmt.Errorf("parse timestamp: %w", err)
  	}
  	return int(pid64), ts, nil
  }

  // removePIDFile deletes the PID file, ignoring "not found" errors.
  func removePIDFile() error {
  	err := os.Remove(daemonPIDFile())
  	if os.IsNotExist(err) {
  		return nil
  	}
  	return err
  }

  // formatUptime formats elapsed seconds as "Xh Ym", "Xm Ys", or "Xs".
  func formatUptime(secs int64) string {
  	if secs < 60 {
  		return fmt.Sprintf("%ds", secs)
  	}
  	if secs < 3600 {
  		m := secs / 60
  		s := secs % 60
  		return fmt.Sprintf("%dm %ds", m, s)
  	}
  	h := secs / 3600
  	m := (secs % 3600) / 60
  	return fmt.Sprintf("%dh %dm", h, m)
  }

  // daemonize re-executes the current binary as a background daemon with the given config path.
  // The caller should return immediately after daemonize returns nil.
  func daemonize(cfgPath string) error {
  	// Check for an existing live daemon.
  	if pid, _, err := readPIDFile(); err == nil {
  		if isAlive(pid) {
  			return fmt.Errorf("daemon already running (PID: %d)", pid)
  		}
  		// Stale file — clean up before spawning.
  		_ = removePIDFile()
  	}

  	// Resolve own executable (never use os.Args[0]).
  	exe, err := os.Executable()
  	if err != nil {
  		return fmt.Errorf("resolve executable: %w", err)
  	}
  	exe, err = filepath.EvalSymlinks(exe)
  	if err != nil {
  		return fmt.Errorf("eval symlinks: %w", err)
  	}

  	// Ensure ~/.openbee/ exists.
  	if err := os.MkdirAll(filepath.Dir(daemonPIDFile()), 0755); err != nil {
  		return fmt.Errorf("create state dir: %w", err)
  	}

  	// Spawn detached child.
  	pid, err := spawnDaemon(exe, []string{"server", "-c", cfgPath}, daemonLogFile())
  	if err != nil {
  		return fmt.Errorf("spawn daemon: %w", err)
  	}
  	// Record start time immediately after spawn, before the liveness poll,
  	// so that status uptime is not understated by the 2 s poll duration.
  	startTS := time.Now().Unix()

  	// Poll child liveness for up to 2 s to catch immediate exits.
  	deadline := time.Now().Add(2 * time.Second)
  	for time.Now().Before(deadline) {
  		time.Sleep(100 * time.Millisecond)
  		if !isAlive(pid) {
  			return fmt.Errorf("daemon process exited immediately (check %s for details)", daemonLogFile())
  		}
  	}

  	// Write PID file only after confirming child is alive.
  	if err := writePIDFile(pid, startTS); err != nil {
  		return fmt.Errorf("write pid file: %w", err)
  	}

  	fmt.Printf("Daemon started (PID: %d)\n", pid)
  	return nil
  }
  ```

- [ ] **Step 4: Commit (tests run after Task 2 creates the platform file)**

  The package requires `daemon_unix.go` or `daemon_windows.go` to compile; tests
  cannot run yet. Commit now and run all Task 1 tests in Task 2 Step 4.

  ```bash
  git add cmd/openbee/daemon.go cmd/openbee/daemon_test.go
  git commit -m "feat: add daemon PID file helpers and shared daemonize logic"
  ```

---

### Task 2: Unix platform implementation (`daemon_unix.go`)

**Files:**
- Create: `cmd/openbee/daemon_unix.go`

- [ ] **Step 1: Write failing test for Unix liveness check**

  Add to `cmd/openbee/daemon_test.go`:

  ```go
  func TestIsAlive(t *testing.T) {
  	// Current process must be alive.
  	assert.True(t, isAlive(os.Getpid()))
  	// An absurdly large PID that cannot exist on any platform.
  	// Note: do NOT test isAlive(0) — on Unix, kill(0, 0) signals the entire process
  	// group and returns success, so isAlive(0) would incorrectly return true.
  	assert.False(t, isAlive(999999999))
  }
  ```

- [ ] **Step 2: Run to confirm it fails (function not defined)**

  ```bash
  go test ./cmd/openbee/ -run TestIsAlive -v
  ```

  Expected: compile error.

- [ ] **Step 3: Create `cmd/openbee/daemon_unix.go`**

  ```go
  //go:build !windows

  package main

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"syscall"
  	"time"
  )

  // spawnDaemon starts exe with args as a detached background process, redirecting
  // stdout and stderr to logFile. Returns the child PID.
  func spawnDaemon(exe string, args []string, logFile string) (int, error) {
  	lf, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  	if err != nil {
  		return 0, fmt.Errorf("open log file: %w", err)
  	}
  	defer lf.Close()

  	env := append(os.Environ(), daemonEnvKey+"=1")
  	cmd := exec.Command(exe, args...)
  	cmd.Env = env
  	cmd.Stdout = lf
  	cmd.Stderr = lf
  	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

  	if err := cmd.Start(); err != nil {
  		return 0, err
  	}
  	// Detach — do not wait on the child.
  	go func() { _ = cmd.Wait() }()
  	return cmd.Process.Pid, nil
  }

  // isAlive reports whether a process with the given PID is running.
  // Uses kill(pid, 0) — the zero-signal POSIX liveness probe.
  func isAlive(pid int) bool {
  	err := syscall.Kill(pid, 0)
  	return err == nil
  }

  // stopProcess sends SIGTERM to pid, waits up to 15 s, then force-kills with SIGKILL.
  func stopProcess(pid int) error {
  	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
  		return fmt.Errorf("send SIGTERM to %d: %w", pid, err)
  	}
  	deadline := time.Now().Add(15 * time.Second)
  	for time.Now().Before(deadline) {
  		time.Sleep(200 * time.Millisecond)
  		if !isAlive(pid) {
  			return nil
  		}
  	}
  	// Graceful shutdown timed out — force kill.
  	_ = syscall.Kill(pid, syscall.SIGKILL)
  	return nil
  }

  // redirectStdio replaces OS file descriptors 1 and 2 with the given log file.
  // Must be called before logger.Init so that zap's os.Stderr sink writes to the log.
  // Uses Dup3 (not Dup2) for Linux/arm64 compatibility. Closes lf after duplicating
  // so the daemon holds exactly one fd per standard stream (fd 1 and fd 2).
  func redirectStdio(logPath string) error {
  	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  	if err != nil {
  		return fmt.Errorf("open log file %s: %w", logPath, err)
  	}
  	fd := int(lf.Fd())
  	if err := syscall.Dup3(fd, 1, 0); err != nil {
  		lf.Close()
  		return fmt.Errorf("dup3 stdout: %w", err)
  	}
  	if err := syscall.Dup3(fd, 2, 0); err != nil {
  		lf.Close()
  		return fmt.Errorf("dup3 stderr: %w", err)
  	}
  	lf.Close() // fd 1 and fd 2 now hold the log file; release the original fd
  	return nil
  }
  ```

- [ ] **Step 4: Run all accumulated tests (Task 1 + Task 2)**

  ```bash
  go test ./cmd/openbee/ -run "TestIsAlive|TestWriteReadPIDFile|TestReadPIDFileMissing|TestIsDaemonChild|TestFormatUptime" -v
  ```

  Expected: 5 tests PASS. This is the first time the package compiles successfully.

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/openbee/daemon_unix.go cmd/openbee/daemon_test.go
  git commit -m "feat: add Unix daemon spawn/signal/liveness implementation"
  ```

---

### Task 3: Windows platform implementation (`daemon_windows.go`)

**Files:**
- Create: `cmd/openbee/daemon_windows.go`

- [ ] **Step 1: Create `cmd/openbee/daemon_windows.go`**

  ```go
  //go:build windows

  package main

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"syscall"
  	"time"

  	"golang.org/x/sys/windows"
  )

  // spawnDaemon starts exe with args as a detached background process on Windows.
  func spawnDaemon(exe string, args []string, logFile string) (int, error) {
  	lf, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  	if err != nil {
  		return 0, fmt.Errorf("open log file: %w", err)
  	}
  	defer lf.Close()

  	env := append(os.Environ(), daemonEnvKey+"=1")
  	cmd := exec.Command(exe, args...)
  	cmd.Env = env
  	cmd.Stdout = lf
  	cmd.Stderr = lf
  	// CREATE_NEW_PROCESS_GROUP: required for GenerateConsoleCtrlEvent targeting.
  	// CREATE_NO_WINDOW: suppresses a new console window.
  	// DETACHED_PROCESS is intentionally not used — it conflicts with CREATE_NEW_PROCESS_GROUP.
  	cmd.SysProcAttr = &syscall.SysProcAttr{
  		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
  	}

  	if err := cmd.Start(); err != nil {
  		return 0, err
  	}
  	go func() { _ = cmd.Wait() }()
  	return cmd.Process.Pid, nil
  }

  // isAlive reports whether a process with the given PID is running on Windows.
  func isAlive(pid int) bool {
  	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
  	if err != nil {
  		return false
  	}
  	defer windows.CloseHandle(h)
  	var code uint32
  	if err := windows.GetExitCodeProcess(h, &code); err != nil {
  		return false
  	}
  	return code == 259 // STILL_ACTIVE
  }

  // stopProcess sends CTRL_BREAK_EVENT to pid for graceful shutdown, then force-kills after 15 s.
  func stopProcess(pid int) error {
  	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)); err != nil {
  		// Fallback: kill immediately if signal delivery fails.
  		p, findErr := os.FindProcess(pid)
  		if findErr != nil {
  			return fmt.Errorf("find process %d: %w", pid, findErr)
  		}
  		return p.Kill()
  	}
  	deadline := time.Now().Add(15 * time.Second)
  	for time.Now().Before(deadline) {
  		time.Sleep(200 * time.Millisecond)
  		if !isAlive(pid) {
  			return nil
  		}
  	}
  	p, err := os.FindProcess(pid)
  	if err != nil {
  		return nil // already gone
  	}
  	return p.Kill()
  }

  // redirectStdio replaces os.Stdout and os.Stderr with the log file on Windows.
  // Also updates the Windows API standard handles so child processes inherit the log file.
  // Must be called before logger.Init. Unlike the Unix version, lf is kept open because
  // os.Stdout and os.Stderr hold a reference to it — zap writes through os.Stderr.
  func redirectStdio(logPath string) error {
  	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  	if err != nil {
  		return fmt.Errorf("open log file %s: %w", logPath, err)
  	}
  	h := windows.Handle(lf.Fd())
  	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, h); err != nil {
  		lf.Close()
  		return fmt.Errorf("set stdout handle: %w", err)
  	}
  	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, h); err != nil {
  		lf.Close()
  		return fmt.Errorf("set stderr handle: %w", err)
  	}
  	// Reassign Go-level vars so os.Stderr is what zap binds to at logger.Init time.
  	// lf is intentionally not closed here — os.Stdout/Stderr hold the reference.
  	os.Stdout = lf
  	os.Stderr = lf
  	return nil
  }
  ```

- [ ] **Step 2: Run `go mod tidy` to promote `golang.org/x/sys` to a direct dependency**

  ```bash
  go mod tidy
  ```

  Verify `go.mod` now lists `golang.org/x/sys` without `// indirect`.

- [ ] **Step 3: Verify the package builds on the current platform (cross-compile check)**

  On macOS/Linux:
  ```bash
  GOOS=windows go build ./cmd/openbee/
  ```

  Expected: compiles without error.

- [ ] **Step 4: Commit**

  ```bash
  git add cmd/openbee/daemon_windows.go go.mod go.sum
  git commit -m "feat: add Windows daemon spawn/signal/liveness implementation"
  ```

---

### Task 4: Modify `server.go` — add `--daemon` flag

**Files:**
- Modify: `cmd/openbee/server.go`

- [ ] **Step 1: Add daemon flag and dispatch logic**

  Replace the contents of `cmd/openbee/server.go` with:

  ```go
  package main

  import (
  	"fmt"

  	"github.com/spf13/cobra"
  	"github.com/theopenbee/openbee/internal/app"
  	"github.com/theopenbee/openbee/internal/config"
  	"github.com/theopenbee/openbee/internal/logger"
  )

  var cfgPath string
  var daemonMode bool

  var serverCmd = &cobra.Command{
  	Use:   "server",
  	Short: "Start the OpenBee server",
  	RunE: func(cmd *cobra.Command, args []string) error {
  		// --- Daemon dispatch ---
  		if daemonMode && !isDaemonChild() {
  			// Parent: spawn background child and exit.
  			return daemonize(cfgPath)
  		}

  		if isDaemonChild() {
  			// Child: redirect stdout+stderr to log file before logger.Init,
  			// so that zap's os.Stderr sink writes to the log file.
  			if err := redirectStdio(daemonLogFile()); err != nil {
  				return fmt.Errorf("redirect stdio: %w", err)
  			}
  			// Clean up PID file on shutdown.
  			defer func() { _ = removePIDFile() }()
  		}

  		// --- Normal server startup ---
  		if err := logger.Init(logger.Config{
  			Level:  "info",
  			Format: "json",
  		}); err != nil {
  			return fmt.Errorf("init logger: %w", err)
  		}
  		cfg, err := config.Load(cfgPath)
  		if err != nil {
  			return fmt.Errorf("load config: %w", err)
  		}

  		a, err := app.BuildApp(cfg)
  		if err != nil {
  			return fmt.Errorf("build app: %w", err)
  		}

  		a.Run()
  		return nil
  	},
  }

  func init() {
  	serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
  	serverCmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, "run as background daemon")
  	rootCmd.AddCommand(serverCmd)
  }
  ```

- [ ] **Step 2: Build the package to verify it compiles**

  ```bash
  go build ./cmd/openbee/
  ```

  Expected: no errors.

- [ ] **Step 3: Commit**

  ```bash
  git add cmd/openbee/server.go
  git commit -m "feat: add --daemon/-d flag to server command"
  ```

---

### Task 5: `stop.go`

**Files:**
- Create: `cmd/openbee/stop.go`

- [ ] **Step 1: Write failing test for stop with no PID file**

  Add to `cmd/openbee/daemon_test.go`:

  ```go
  func TestStopNotRunning(t *testing.T) {
  	dir := t.TempDir()
  	pidPath := filepath.Join(dir, "openbee.pid")
  	// File does not exist — doStop should succeed (exit 0 semantics).
  	err := doStop(pidPath)
  	assert.NoError(t, err)
  }
  ```

- [ ] **Step 2: Create `cmd/openbee/stop.go`**

  ```go
  package main

  import (
  	"fmt"
  	"os"

  	"github.com/spf13/cobra"
  )

  var stopCmd = &cobra.Command{
  	Use:   "stop",
  	Short: "Stop the running OpenBee daemon",
  	RunE: func(cmd *cobra.Command, args []string) error {
  		return doStop(daemonPIDFile())
  	},
  }

  // doStop is the testable core of stopCmd. pidFile is injected for hermetic testing.
  func doStop(pidFile string) error {
  	pid, _, err := readPIDFileFrom(pidFile)
  	if err != nil {
  		// No PID file — daemon is not running.
  		fmt.Println("openbee is not running")
  		return nil
  	}

  	if !isAlive(pid) {
  		fmt.Println("openbee is not running (stale PID file removed)")
  		return os.Remove(pidFile)
  	}

  	fmt.Printf("Stopping openbee (PID: %d)...\n", pid)
  	if err := stopProcess(pid); err != nil {
  		return fmt.Errorf("stop process: %w", err)
  	}

  	// Daemon removes PID file on clean exit; remove it here if still present.
  	_ = os.Remove(pidFile)
  	fmt.Println("openbee stopped")
  	return nil
  }

  func init() {
  	rootCmd.AddCommand(stopCmd)
  }
  ```

- [ ] **Step 3: Run tests**

  ```bash
  go test ./cmd/openbee/ -run "TestStopNotRunning" -v
  ```

  Expected: PASS.

- [ ] **Step 4: Build to verify**

  ```bash
  go build ./cmd/openbee/
  ```

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/openbee/stop.go cmd/openbee/daemon_test.go
  git commit -m "feat: add stop command"
  ```

---

### Task 6: `status.go`

**Files:**
- Create: `cmd/openbee/status.go`

- [ ] **Step 1: Write failing test for status with no PID file**

  Add to `cmd/openbee/daemon_test.go`:

  ```go
  func TestStatusOutput(t *testing.T) {
  	dir := t.TempDir()
  	path := filepath.Join(dir, "test.pid")

  	// Not running case — missing file.
  	running, msg := daemonStatus(path)
  	assert.False(t, running)
  	assert.Contains(t, msg, "not running")

  	// Write a PID file for current process (definitely alive).
  	ts := time.Now().Add(-90 * time.Second).Unix() // 1m 30s ago
  	require.NoError(t, writePIDFileTo(path, os.Getpid(), ts))

  	running, msg = daemonStatus(path)
  	assert.True(t, running)
  	assert.Contains(t, msg, fmt.Sprintf("%d", os.Getpid()))
  	assert.Contains(t, msg, "running")
  }
  ```

- [ ] **Step 2: Create `cmd/openbee/status.go`**

  ```go
  package main

  import (
  	"fmt"
  	"os"
  	"time"

  	"github.com/spf13/cobra"
  )

  var statusCmd = &cobra.Command{
  	Use:   "status",
  	Short: "Show the status of the OpenBee daemon",
  	RunE: func(cmd *cobra.Command, args []string) error {
  		running, msg := daemonStatus(daemonPIDFile())
  		fmt.Println(msg)
  		if !running {
  			os.Exit(1)
  		}
  		return nil
  	},
  }

  // daemonStatus returns whether the daemon is running and a human-readable status string.
  // pidFilePath is injected to allow testing without touching the real PID file.
  func daemonStatus(pidFilePath string) (running bool, msg string) {
  	pid, startTS, err := readPIDFileFrom(pidFilePath)
  	if err != nil {
  		return false, "○ openbee is not running"
  	}

  	if !isAlive(pid) {
  		_ = os.Remove(pidFilePath) // clean up stale file
  		return false, "○ openbee is not running (stale PID file removed)"
  	}

  	uptime := time.Now().Unix() - startTS
  	return true, fmt.Sprintf("● openbee is running   (PID: %d, uptime: %s)", pid, formatUptime(uptime))
  }

  func init() {
  	rootCmd.AddCommand(statusCmd)
  }
  ```

- [ ] **Step 3: Run tests**

  ```bash
  go test ./cmd/openbee/ -run "TestStatusOutput|TestFormatUptime" -v
  ```

  Expected: all PASS.

- [ ] **Step 4: Commit**

  ```bash
  git add cmd/openbee/status.go cmd/openbee/daemon_test.go
  git commit -m "feat: add status command"
  ```

---

### Task 7: `restart.go`

**Files:**
- Create: `cmd/openbee/restart.go`

- [ ] **Step 1: Create `cmd/openbee/restart.go`**

  ```go
  package main

  import (
  	"github.com/spf13/cobra"
  )

  var restartCfgPath string

  var restartCmd = &cobra.Command{
  	Use:   "restart",
  	Short: "Restart the OpenBee daemon",
  	RunE: func(cmd *cobra.Command, args []string) error {
  		// Stop the existing daemon (tolerates not-running).
  		if err := doStop(daemonPIDFile()); err != nil {
  			return err
  		}
  		// Spawn a fresh daemon with the given config.
  		return daemonize(restartCfgPath)
  	},
  }

  func init() {
  	restartCmd.Flags().StringVarP(&restartCfgPath, "config", "c", "config.yaml", "path to config file")
  	rootCmd.AddCommand(restartCmd)
  }
  ```

- [ ] **Step 2: Build to verify**

  ```bash
  go build ./cmd/openbee/
  ```

- [ ] **Step 3: Commit**

  ```bash
  git add cmd/openbee/restart.go
  git commit -m "feat: add restart command"
  ```

---

### Task 8: Run all tests and verify cross-compile

- [ ] **Step 1: Run the full test suite**

  ```bash
  cd /Users/tengyongzhi/work/bot-workspaces/openbee
  go test ./... -v 2>&1 | tail -40
  ```

  Expected: all existing tests plus new daemon tests PASS. No regressions.

- [ ] **Step 2: Cross-compile for Windows**

  ```bash
  GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/openbee/
  ```

  Expected: compiles without error.

- [ ] **Step 3: Verify CLI help shows new commands**

  ```bash
  go run ./cmd/openbee/ --help
  go run ./cmd/openbee/ server --help
  ```

  Expected output includes: `stop`, `restart`, `status` in the commands list; `server --help` shows `-d, --daemon` flag.

- [ ] **Step 4: Final commit if any stray changes**

  ```bash
  git status
  # If clean, nothing to do.
  ```
