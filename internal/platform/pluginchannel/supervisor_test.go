package pluginchannel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noteRecorder struct {
	mu    sync.Mutex
	notes []notification
}

func (r *noteRecorder) handle(method string, params json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes = append(r.notes, notification{method, params})
}

func (r *noteRecorder) snapshot() []notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notification(nil), r.notes...)
}

func testSupervisor(t *testing.T, mode string, onNotify NotificationHandler, extraEnv ...string) *Supervisor {
	t.Helper()
	s := NewSupervisor(Spec{
		ChannelID: "fake",
		Dir:       t.TempDir(),
		Command:   fakePluginCommand(),
		Env:       fakePluginEnv(mode, extraEnv...),
		Init:      InitializeParams{ProtocolVersion: 1, ChannelID: "fake", Config: map[string]any{"token": "secret-x"}},
	}, onNotify)
	s.initialBackoff = 10 * time.Millisecond
	s.maxBackoff = 40 * time.Millisecond
	s.shutdownTimeout = time.Second
	s.killGrace = 500 * time.Millisecond
	return s
}

func runSupervisor(t *testing.T, s *Supervisor) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.Done():
		case <-time.After(10 * time.Second):
			t.Error("supervisor did not stop")
		}
	})
	return cancel
}

func waitRunning(t *testing.T, s *Supervisor) {
	t.Helper()
	require.Eventually(t, func() bool {
		err := s.Call(context.Background(), "send", SendParams{SessionKey: "fake:probe:x"}, nil)
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)
}

func TestSupervisor_InitializeAndCall(t *testing.T) {
	rec := &noteRecorder{}
	initFile := filepath.Join(t.TempDir(), "init.json")
	s := testSupervisor(t, "ok", rec.handle, "FAKE_PLUGIN_INITFILE="+initFile)
	runSupervisor(t, s)
	waitRunning(t, s)

	require.NoError(t, s.Call(context.Background(), "send", SendParams{SessionKey: "fake:c1:u1", Content: "hello"}, nil))
	require.Eventually(t, func() bool {
		for _, n := range rec.snapshot() {
			if n.method == "inbound" && strings.Contains(string(n.params), "hello") {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	for _, n := range rec.snapshot() {
		assert.NotEqual(t, "log", n.method)
	}

	data, err := os.ReadFile(initFile)
	require.NoError(t, err)
	var init InitializeParams
	require.NoError(t, json.Unmarshal(data, &init))
	assert.Equal(t, "fake", init.ChannelID)
	assert.Equal(t, "secret-x", init.Config["token"])
}

func assertStopsOnItsOwn(t *testing.T, s *Supervisor) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor kept running after a permanent failure")
	}
	assert.ErrorIs(t, s.Call(context.Background(), "send", nil, nil), ErrNotRunning)
}

func TestSupervisor_ConfigInvalidIsPermanent(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "starts")
	s := testSupervisor(t, "config_invalid", nil, "FAKE_PLUGIN_COUNTER="+counter)
	assertStopsOnItsOwn(t, s)
	data, err := os.ReadFile(counter)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "start"))
}

func TestSupervisor_ProtocolMismatchIsPermanent(t *testing.T) {
	assertStopsOnItsOwn(t, testSupervisor(t, "bad_protocol", nil))
}

func TestSupervisor_MissingCommandIsPermanent(t *testing.T) {
	s := testSupervisor(t, "ok", nil)
	s.spec.Command = []string{"definitely-not-a-binary-xyz"}
	assertStopsOnItsOwn(t, s)
}

func TestSupervisor_MissingRelativeCommandIsPermanent(t *testing.T) {
	s := testSupervisor(t, "ok", nil)
	s.spec.Command = []string{"./bin/missing"}
	assertStopsOnItsOwn(t, s)
}

func TestSupervisor_RestartsWithBackoff(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "starts")
	s := testSupervisor(t, "count_starts", nil, "FAKE_PLUGIN_COUNTER="+counter)
	runSupervisor(t, s)
	require.Eventually(t, func() bool {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "start") >= 3
	}, 10*time.Second, 20*time.Millisecond)
}

func TestSupervisor_CallBeforeRun(t *testing.T) {
	s := testSupervisor(t, "ok", nil)
	err := s.Call(context.Background(), "send", nil, nil)
	assert.True(t, errors.Is(err, ErrNotRunning))
	assert.Contains(t, err.Error(), "fake")
}

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
	if runtime.GOOS == "windows" {
		t.Skip("unix process semantics")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("unix process semantics")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("unix process semantics")
	}
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
