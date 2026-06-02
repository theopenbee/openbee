//go:build !windows

package daemoncmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/theopenbee/openbee/internal/infra/utils"
)

// spawnDaemon starts exe with args as a detached background process, redirecting
// stdout and stderr to logFile. Returns the child PID.
// The goroutine reap call is present for correctness; the parent exits shortly after.
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
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

// isPIDForeign reports whether a process with the given PID exists but is owned
// by a different user (EPERM from kill(pid, 0)). This distinguishes "PID was
// recycled by another user's process" from "process does not exist at all".
func isPIDForeign(pid int) bool {
	return syscall.Kill(pid, 0) == syscall.EPERM
}

// stopProcess sends SIGTERM to pid, waits up to 15 s, then force-kills with SIGKILL.
func stopProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to %d: %w", pid, err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if !utils.IsProcessAlive(pid) {
			return nil
		}
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return nil
}
