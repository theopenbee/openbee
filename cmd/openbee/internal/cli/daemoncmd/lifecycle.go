package daemoncmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const (
	daemonEnvKey   = "OPENBEE_DAEMON"
	daemonEnvValue = "1"
)

// ExitCodeFunc is a function that converts an integer exit code into an error
// value that the CLI runner recognises and converts to os.Exit(code).
type ExitCodeFunc func(int) error

// isDaemonChild reports whether this process was launched as a daemon child.
func isDaemonChild() bool {
	return os.Getenv(daemonEnvKey) == daemonEnvValue
}

func writePIDFileTo(path string, pid int, startTS int64) error {
	content := fmt.Sprintf("%d\n%d\n", pid, startTS)
	return os.WriteFile(path, []byte(content), 0600)
}

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

func removePIDFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// formatUptime formats elapsed seconds as "Xh Ym", "Xm Ys", or "Xs".
func formatUptime(secs int64) string {
	if secs < 0 {
		return "0s"
	}
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
	stateDir, err := config.OpenbeeHomeDir()
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}
	pidFile, err := config.DaemonPIDFile()
	if err != nil {
		return err
	}
	logFile, err := config.DaemonLogFile()
	if err != nil {
		return err
	}

	// Check for an existing live daemon.
	if pid, _, err := readPIDFileFrom(pidFile); err == nil {
		if utils.IsProcessAlive(pid) {
			return fmt.Errorf(i18n.M.Output.Daemon.AlreadyRunning, pid)
		}
		// Stale file — clean up before spawning.
		_ = os.Remove(pidFile)
	}

	// Resolve own executable (never use os.Args[0]).
	exe, err := utils.ResolveExecutable()
	if err != nil {
		return err
	}

	// Ensure ~/.openbee/ exists.
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	// Spawn detached child.
	pid, err := spawnDaemon(exe, []string{"server", "-c", cfgPath}, logFile)
	if err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	// Record start time immediately after spawn, before the liveness poll,
	// so that status uptime is not understated by the poll duration.
	startTS := time.Now().Unix()

	// Wait briefly, then verify the child is still alive (catches immediate crashes).
	time.Sleep(100 * time.Millisecond)
	if !utils.IsProcessAlive(pid) {
		return fmt.Errorf("daemon process exited immediately (check %s for details)", logFile)
	}

	// Write PID file only after confirming child is alive.
	if err := writePIDFileTo(pidFile, pid, startTS); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	fmt.Printf(i18n.M.Output.Daemon.Started+"\n", pid)
	return nil
}
