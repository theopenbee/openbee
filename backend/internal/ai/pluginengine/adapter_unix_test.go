//go:build !windows

package pluginengine

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

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
