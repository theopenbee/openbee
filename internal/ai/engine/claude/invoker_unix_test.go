//go:build !windows

package claude

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestInvoker_Run_ProcessIsInOwnGroup(t *testing.T) {
	// Create a wrapper script that ignores the claude-specific CLI args and just sleeps.
	// This lets us verify the invoker sets Setpgid via PGID == PID.
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "dummy.sh")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nsleep 10000\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	inv := NewInvoker(wrapper, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logPath := filepath.Join(dir, "out.log")
	proc, ch, err := inv.Run(ctx, dir, "prompt", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer func() {
		proc.Stop() //nolint:errcheck
		for range ch {
		}
	}()

	time.Sleep(20 * time.Millisecond)

	pid := proc.PID()
	if pid == 0 {
		t.Fatal("PID is 0")
	}

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Errorf("want pgid==pid (%d), got pgid=%d — ConfigureCmd not called in invoker", pid, pgid)
	}
}
