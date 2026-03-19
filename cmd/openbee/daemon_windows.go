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
