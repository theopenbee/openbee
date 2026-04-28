//go:build !windows

package ai

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestConfigureCmd_SetsPgid(t *testing.T) {
	cmd := exec.Command("true")
	ConfigureCmd(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil after ConfigureCmd")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("Setpgid should be true after ConfigureCmd")
	}
}

func TestCmdProcess_Stop_KillsProcessGroup(t *testing.T) {
	// Write child PID to temp file so we can check it after Stop().
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf("sleep 10000 & echo $! > %s; wait", pidFile)

	cmd := exec.Command("sh", "-c", script)
	ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	proc := NewCmdProcess(cmd)

	// Wait for the child PID file to appear.
	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				childPID = pid
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child PID was not written to file within 2 seconds")
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cmd.Wait() //nolint:errcheck

	// Allow brief time for OS to reap the child.
	time.Sleep(50 * time.Millisecond)

	// kill -0 returns nil if the process still exists.
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Errorf("child process %d still alive after Stop() — process group kill failed", childPID)
	}
}
