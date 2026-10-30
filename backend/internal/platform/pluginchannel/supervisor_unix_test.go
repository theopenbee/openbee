//go:build !windows

package pluginchannel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pidFromFile(t *testing.T, path string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	}, 10*time.Second, 10*time.Millisecond)
	return pid
}

func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestSupervisor_ShutdownKillsStubbornPlugin(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	s := testSupervisor(t, "ignore_shutdown", nil, "FAKE_PLUGIN_PIDFILE="+pidFile)
	s.shutdownTimeout = 100 * time.Millisecond
	s.killGrace = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	waitRunning(t, s)
	pid := pidFromFile(t, pidFile)

	cancel()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}
	assert.Eventually(t, func() bool { return processGone(pid) }, 2*time.Second, 10*time.Millisecond)
}

func TestSupervisor_GracefulShutdown(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	s := testSupervisor(t, "ok", nil, "FAKE_PLUGIN_PIDFILE="+pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	waitRunning(t, s)
	pid := pidFromFile(t, pidFile)

	start := time.Now()
	cancel()
	<-s.Done()
	assert.Less(t, time.Since(start), s.killGrace)
	assert.True(t, processGone(pid))
}

func TestSupervisor_PluginExitsWhenStdinCloses(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	host := exec.Command(os.Args[0], "-test.run=^$")
	host.Env = append(os.Environ(), "OPENBEE_FAKE_HOST=1", "FAKE_HOST_PLUGIN_PIDFILE="+pidFile)
	require.NoError(t, host.Start())
	pluginPID := pidFromFile(t, pidFile)
	require.False(t, processGone(pluginPID))

	require.NoError(t, host.Process.Signal(syscall.SIGKILL))
	_ = host.Wait()

	assert.Eventually(t, func() bool { return processGone(pluginPID) }, 5*time.Second, 20*time.Millisecond,
		"plugin outlived its killed host")
}
