//go:build !windows

package pluginengine

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const sweepWarning = "kill engine plugin process group"

func observedSpecAdapter(spec Spec) (*Adapter, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	a := New(spec)
	a.log = zap.New(core)
	return a, logs
}

func assertNoSweepWarning(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	assert.Empty(t, logs.FilterMessage(sweepWarning).All(), "a stopped or cancelled process group must not log a kill warning")
}

func waitChildPID(t *testing.T, dir string) int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, rec := range readRecords(t, dir) {
			if rec.ChildPID != 0 {
				return rec.ChildPID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fake engine never reported its child PID")
	return 0
}

func assertGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("child process %d is still alive", pid)
}

func startSleeper(t *testing.T, ctx context.Context) (ai.RunResult, int) {
	t.Helper()
	spec, recordDir := fakeSpec(t, "sleep-with-child")
	res, err := New(spec).Run(ctx, t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })
	child := waitChildPID(t, recordDir)

	pid := res.Process.PID()
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	assert.Equal(t, pid, pgid, "engine plugin must lead its own process group")
	childPgid, err := syscall.Getpgid(child)
	require.NoError(t, err)
	assert.Equal(t, pid, childPgid, "CLI child must stay in the plugin's process group")
	return res, child
}

func TestRun_StopKillsGroup(t *testing.T) {
	res, child := startSleeper(t, context.Background())

	require.NoError(t, res.Process.Stop())
	out := awaitOutput(t, res)
	assert.Equal(t, ai.OutputError, out.Type)
	assert.NotEmpty(t, out.Content)
	assertGone(t, child)
}

func TestRun_ContextCancelKillsGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, child := startSleeper(t, ctx)

	cancel()
	out := awaitOutput(t, res)
	assert.Equal(t, ai.OutputError, out.Type)
	assertGone(t, child)
}

func TestRun_FailedRunKillsGroup(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"child-noresp", "engine plugin exited without a response: cli crashed"},
		{"child-error", "cli failed"},
		{"child-holds-stdout", "engine plugin exited without a response"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			spec, recordDir := fakeSpec(t, c.mode)
			res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
			require.NoError(t, err)
			t.Cleanup(func() { _ = res.Process.Stop() })
			child := waitChildPID(t, recordDir)

			assert.Equal(t, ai.Output{Type: ai.OutputError, Content: c.want}, awaitOutput(t, res))
			assertGone(t, child)
		})
	}
}

func TestCheck_FailedCallKillsGroup(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"child-noresp", "engine plugin exited without a response: cli crashed"},
		{"child-error", "cli failed"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			spec, recordDir := fakeSpec(t, c.mode)
			_, err := New(spec).Check(context.Background())
			require.EqualError(t, err, c.want)
			assertGone(t, waitChildPID(t, recordDir))
		})
	}
}

func TestRun_SuccessKeepsBackgroundChildren(t *testing.T) {
	spec, recordDir := fakeSpec(t, "child-ok")
	res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })

	assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))
	child := waitChildPID(t, recordDir)
	time.Sleep(300 * time.Millisecond)
	assert.NoError(t, syscall.Kill(child, 0), "a successful run must leave background processes alone")
}

func TestRun_KilledAfterResponseFails(t *testing.T) {
	spec, recordDir := fakeSpec(t, "respond-then-block")
	res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })
	child := waitChildPID(t, recordDir)

	require.NoError(t, res.Process.Stop())
	assert.Equal(t, ai.Output{Type: ai.OutputError, Content: "signal: killed"}, awaitOutput(t, res))
	assertGone(t, child)
}

func TestRun_StopDoesNotWarnAboutSweep(t *testing.T) {
	spec, recordDir := fakeSpec(t, "sleep-with-child")
	a, logs := observedSpecAdapter(spec)
	res, err := a.Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })
	child := waitChildPID(t, recordDir)

	require.NoError(t, res.Process.Stop())
	assert.Equal(t, ai.OutputError, awaitOutput(t, res).Type)
	assertGone(t, child)
	assertNoSweepWarning(t, logs)
}

func TestRun_ContextCancelDoesNotWarnAboutSweep(t *testing.T) {
	spec, recordDir := fakeSpec(t, "sleep-with-child")
	a, logs := observedSpecAdapter(spec)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := a.Run(ctx, t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })
	child := waitChildPID(t, recordDir)

	cancel()
	assert.Equal(t, ai.OutputError, awaitOutput(t, res).Type)
	assertGone(t, child)
	assertNoSweepWarning(t, logs)
}

func TestRun_TimeoutDoesNotWarnAboutSweep(t *testing.T) {
	spec, recordDir := fakeSpec(t, "sleep-with-child")
	a, logs := observedSpecAdapter(spec)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	res, err := a.Run(ctx, t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Process.Stop() })
	child := waitChildPID(t, recordDir)

	assert.Equal(t, ai.OutputError, awaitOutput(t, res).Type)
	assertGone(t, child)
	assertNoSweepWarning(t, logs)
}

func TestRun_FailedRunWithoutLeftoversDoesNotWarnAboutSweep(t *testing.T) {
	spec, _ := fakeSpec(t, "run-error")
	a, logs := observedSpecAdapter(spec)
	res, err := a.Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)

	assert.Equal(t, ai.Output{Type: ai.OutputError, Content: "fakecli exited with code 2"}, awaitOutput(t, res))
	assertNoSweepWarning(t, logs)
}

func TestCheck_TimeoutDoesNotWarnAboutSweep(t *testing.T) {
	spec, recordDir := fakeSpec(t, "sleep-with-child")
	a, logs := observedSpecAdapter(spec)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := a.Check(ctx)
	require.Error(t, err)
	assertGone(t, waitChildPID(t, recordDir))
	assertNoSweepWarning(t, logs)
}

func newSweepSubject(t *testing.T) (*pluginProcess, *observer.ObservedLogs) {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	ai.ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	core, logs := observer.New(zapcore.DebugLevel)
	return &pluginProcess{cmd: cmd, proc: ai.NewCmdProcess(cmd), log: zap.New(core)}, logs
}

func TestSweep_SkipsAfterStop(t *testing.T) {
	p, logs := newSweepSubject(t)

	require.NoError(t, p.proc.Stop())
	time.Sleep(200 * time.Millisecond)
	p.sweep()

	assertNoSweepWarning(t, logs)
}

func TestSweep_KillsGroupThatWasNotStopped(t *testing.T) {
	p, logs := newSweepSubject(t)

	p.sweep()

	assertNoSweepWarning(t, logs)
	assert.EqualError(t, p.cmd.Wait(), "signal: killed")
}

func TestSweep_IgnoresDeniedSignalOnExitedLeader(t *testing.T) {
	cmd := exec.Command("true")
	ai.ConfigureCmd(cmd)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Wait() })
	time.Sleep(300 * time.Millisecond)
	core, logs := observer.New(zapcore.DebugLevel)
	p := &pluginProcess{cmd: cmd, proc: ai.NewCmdProcess(cmd), log: zap.New(core)}

	p.sweep()

	assertNoSweepWarning(t, logs)
}
