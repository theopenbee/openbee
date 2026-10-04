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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/utils"
)

func TestConfigureCmd_SetsPgid(t *testing.T) {
	cmd := exec.Command("true")
	ConfigureCmd(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid)
}

func TestCmdProcess_Stop_KillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf("sleep 10000 & echo $! > %s; wait", pidFile)

	cmd := exec.Command("sh", "-c", script)
	ConfigureCmd(cmd)

	require.NoError(t, cmd.Start())

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
	require.NotZero(t, childPID, "child PID was not written to file within 2 seconds")

	require.NoError(t, proc.Stop())
	cmd.Wait() //nolint:errcheck

	time.Sleep(50 * time.Millisecond)

	assert.False(t, utils.IsProcessAlive(childPID), "child process %d still alive after Stop() — process group kill failed", childPID)
}
