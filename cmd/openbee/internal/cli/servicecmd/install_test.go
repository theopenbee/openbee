package servicecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestInstall_DefaultAutoStart(t *testing.T) {
	fm := &fakeManager{}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(fm.installCalls) != 1 {
		t.Fatalf("Install called %d times", len(fm.installCalls))
	}
	if !fm.installCalls[0].AutoStart {
		t.Errorf("AutoStart should default to true")
	}
	if fm.installCalls[0].Force {
		t.Errorf("Force should default to false")
	}
}

func TestInstall_NoStart(t *testing.T) {
	fm := &fakeManager{}
	withFake(t, fm)
	cfg := writeFakeConfig(t)

	cmd := NewCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--config", cfg, "--no-start"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fm.installCalls[0].AutoStart {
		t.Errorf("AutoStart should be false with --no-start")
	}
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
	cmd.SetArgs([]string{"install", "--config", cfg})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
