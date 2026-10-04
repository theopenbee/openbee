package servicecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFastVerify(t *testing.T) {
	t.Helper()
	prevTimeout := verifyRunningTimeout
	prevPoll := verifyRunningPoll
	verifyRunningTimeout = 30 * time.Millisecond
	verifyRunningPoll = 5 * time.Millisecond
	t.Cleanup(func() {
		verifyRunningTimeout = prevTimeout
		verifyRunningPoll = prevPoll
	})
}

type fakeManager struct {
	installCalls []InstallOptions
	installErr   error
	uninstallErr error
	startErr     error
	stopErr      error
	status       Status
	statusErr    error
}

func (f *fakeManager) Install(_ context.Context, opts InstallOptions) error {
	f.installCalls = append(f.installCalls, opts)
	return f.installErr
}
func (f *fakeManager) Uninstall(context.Context) error        { return f.uninstallErr }
func (f *fakeManager) Start(context.Context) error            { return f.startErr }
func (f *fakeManager) Stop(context.Context) error             { return f.stopErr }
func (f *fakeManager) Status(context.Context) (Status, error) { return f.status, f.statusErr }

func withFake(t *testing.T, fm *fakeManager) {
	t.Helper()
	prev := newManager
	newManager = func() (Manager, error) { return fm, nil }
	t.Cleanup(func() { newManager = prev })
}

func writeFakeConfig(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	return cfg
}

func TestInstall_DefaultAutoStart(t *testing.T) {
	withFastVerify(t)
	fm := &fakeManager{status: Status{RunState: RunStateRunning}}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg, "--run-as", currentUsername(t)})
	require.NoError(t, cmd.Execute())

	require.Len(t, fm.installCalls, 1)
	assert.True(t, fm.installCalls[0].AutoStart, "AutoStart should default to true")
	assert.False(t, fm.installCalls[0].Force, "Force should default to false")
}

func TestInstall_VerifyFailsWhenNotRunning(t *testing.T) {
	withFastVerify(t)
	fm := &fakeManager{status: Status{RunState: RunStateStopped, LastExitCode: "78"}}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"install", "--config", cfg, "--run-as", currentUsername(t)})
	require.Error(t, cmd.Execute(), "expected error when post-install verification fails")
	assert.Contains(t, out.String(), "78")
}

func TestStart_SuccessWhenRunning(t *testing.T) {
	withFastVerify(t)
	fm := &fakeManager{status: Status{RunState: RunStateRunning, PID: 4242}}
	withFake(t, fm)

	cmd := NewCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"start"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "4242")
}

func TestStart_FailureWhenStopped(t *testing.T) {
	withFastVerify(t)
	fm := &fakeManager{status: Status{RunState: RunStateStopped, LastExitReason: "SIGSEGV"}}
	withFake(t, fm)

	cmd := NewCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"start"})
	require.Error(t, cmd.Execute(), "expected error when start verification fails")
	assert.Contains(t, out.String(), "SIGSEGV")
}

func TestInstall_NoStart(t *testing.T) {
	fm := &fakeManager{}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg, "--no-start", "--run-as", currentUsername(t)})
	require.NoError(t, cmd.Execute())
	assert.False(t, fm.installCalls[0].AutoStart, "AutoStart should be false with --no-start")
}

func TestInstall_ManagerError(t *testing.T) {
	fm := &fakeManager{installErr: errors.New("boom")}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"install", "--config", cfg, "--run-as", currentUsername(t)})
	require.Error(t, cmd.Execute())
}
