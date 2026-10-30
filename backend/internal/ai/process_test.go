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

func TestCmdProcess_Stop_IsIdempotent(t *testing.T) {
	cmd := exec.Command("sleep", "10000")
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	proc := NewCmdProcess(cmd)

	require.NoError(t, proc.Stop())
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, proc.Stop())
	assert.EqualError(t, cmd.Wait(), "signal: killed")
	require.NoError(t, proc.Stop())
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

func TestCmdProcess_Terminate_LetsTheGroupCleanUp(t *testing.T) {
	dir := t.TempDir()
	ready, seen := filepath.Join(dir, "ready"), filepath.Join(dir, "seen")
	cmd := exec.Command("sh", "-c", fmt.Sprintf(`trap 'touch %s; exit 0' TERM; touch %s; while :; do sleep 0.02; done`, seen, ready))
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	waitForFile(t, ready)

	require.NoError(t, NewCmdProcess(cmd).Terminate(time.Minute))

	require.NoError(t, cmd.Wait())
	assert.FileExists(t, seen)
}

func TestCmdProcess_Terminate_KillsAfterGrace(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	cmd := exec.Command("sh", "-c", fmt.Sprintf(`trap '' TERM; touch %s; while :; do sleep 0.02; done`, ready))
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	waitForFile(t, ready)

	start := time.Now()
	require.NoError(t, NewCmdProcess(cmd).Terminate(200*time.Millisecond))

	assert.EqualError(t, cmd.Wait(), "signal: killed")
	assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond)
}

func TestCmdProcess_Terminate_KillsSurvivorsAfterLeaderExits(t *testing.T) {
	dir := t.TempDir()
	ready, pidFile := filepath.Join(dir, "ready"), filepath.Join(dir, "pid")
	script := fmt.Sprintf(`sh -c 'trap "" TERM; echo $$ > %s; touch %s; while :; do sleep 0.02; done' & trap 'exit 0' TERM; while :; do sleep 0.02; done`, pidFile, ready)
	cmd := exec.Command("sh", "-c", script)
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	waitForFile(t, ready)
	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	child, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)

	require.NoError(t, NewCmdProcess(cmd).Terminate(200*time.Millisecond))
	require.NoError(t, cmd.Wait())

	assert.True(t, utils.IsProcessAlive(child), "the survivor must get the grace period")
	assert.Eventually(t, func() bool { return !utils.IsProcessAlive(child) }, 3*time.Second, 20*time.Millisecond)
}

func TestCmdProcess_Terminate_IsIdempotent(t *testing.T) {
	cmd := exec.Command("sleep", "10000")
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	proc := NewCmdProcess(cmd)

	require.NoError(t, proc.Terminate(time.Minute))
	require.NoError(t, proc.Terminate(time.Minute))
	assert.EqualError(t, cmd.Wait(), "signal: terminated")
	require.NoError(t, proc.Terminate(time.Minute))
}

func TestCmdProcess_Terminate_AfterStopIsANoOp(t *testing.T) {
	cmd := exec.Command("sleep", "10000")
	ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	proc := NewCmdProcess(cmd)

	require.NoError(t, proc.Stop())
	assert.EqualError(t, cmd.Wait(), "signal: killed")
	require.NoError(t, proc.Terminate(time.Minute))
}
