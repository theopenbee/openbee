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
