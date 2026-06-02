//go:build !windows

package ai

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/utils"
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
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf("sleep 10000 & echo $! > %s; wait", pidFile)

	cmd := exec.Command("sh", "-c", script)
	ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	proc := NewCmdProcess(cmd)

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

	time.Sleep(50 * time.Millisecond)

	if utils.IsProcessAlive(childPID) {
		t.Errorf("child process %d still alive after Stop() — process group kill failed", childPID)
	}
}
