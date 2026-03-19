package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAlive(t *testing.T) {
	// Current process must be alive.
	assert.True(t, isAlive(os.Getpid()))
	// An absurdly large PID that cannot exist on any platform.
	// Note: do NOT test isAlive(0) — on Unix, kill(0, 0) signals the entire process
	// group and returns success, so isAlive(0) would incorrectly return true.
	assert.False(t, isAlive(999999999))
}

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
	t.Setenv("OPENBEE_DAEMON", "")
	assert.False(t, isDaemonChild())

	t.Setenv("OPENBEE_DAEMON", "1")
	assert.True(t, isDaemonChild())
}

func TestFormatUptime(t *testing.T) {
	assert.Equal(t, "0s", formatUptime(-1))
	assert.Equal(t, "0s", formatUptime(0))
	assert.Equal(t, "45s", formatUptime(45))
	assert.Equal(t, "5m 3s", formatUptime(303))
	assert.Equal(t, "2h 5m", formatUptime(7530))
	assert.Equal(t, "25h 0m", formatUptime(90000))
}

func TestStopNotRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "openbee.pid")
	// File does not exist — doStop should succeed (exit 0 semantics).
	err := doStop(pidPath)
	assert.NoError(t, err)
}

func TestStopStalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "openbee.pid")
	// Write a PID that cannot possibly be alive (absurdly large).
	require.NoError(t, writePIDFileTo(pidPath, 999999999, time.Now().Unix()))
	err := doStop(pidPath)
	assert.NoError(t, err)
	// PID file must have been removed.
	_, statErr := os.Stat(pidPath)
	assert.True(t, os.IsNotExist(statErr), "stale PID file should be removed after stop")
}

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
