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
	home, err := os.UserHomeDir()
	if err != nil {
		panic("daemon: cannot determine home directory: " + err.Error())
	}
	return filepath.Join(home, ".openbee", "openbee.pid")
}

// daemonLogFile returns the path to the daemon log file.
func daemonLogFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("daemon: cannot determine home directory: " + err.Error())
	}
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
	return os.WriteFile(path, []byte(content), 0600)
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
