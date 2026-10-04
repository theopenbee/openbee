package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsProcessAlive_Self(t *testing.T) {
	require.True(t, IsProcessAlive(os.Getpid()), "current process must be alive")
}

func TestIsProcessAlive_InvalidPID(t *testing.T) {
	require.False(t, IsProcessAlive(0), "pid 0 must not be reported alive")
	require.False(t, IsProcessAlive(-1), "negative pid must not be reported alive")
}

func TestIsProcessAlive_LikelyDeadPID(t *testing.T) {
	const pid = 999999
	if IsProcessAlive(pid) {
		t.Skip("pid 999999 happened to exist")
	}
}
