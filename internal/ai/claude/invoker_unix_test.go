//go:build !windows

package claude

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestInvoker_Run_ProcessIsInOwnGroup(t *testing.T) {
	// Create a wrapper script that ignores the claude-specific CLI args and just sleeps.
	// This lets us verify the invoker sets Setpgid via PGID == PID.
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "dummy.sh")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nsleep 10000\n"), 0o755))

	inv := NewInvoker(wrapper, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logPath := filepath.Join(dir, "out.log")
	proc, ch, err := inv.Run(ctx, dir, "prompt", ai.RunOptions{}, logPath)
	require.NoError(t, err)
	defer func() {
		proc.Stop() //nolint:errcheck
		for range ch {
		}
	}()

	time.Sleep(20 * time.Millisecond)

	pid := proc.PID()
	require.NotZero(t, pid)

	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	assert.Equal(t, pid, pgid, "ConfigureCmd not called in invoker")
}
