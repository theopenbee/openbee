//go:build linux

package servicecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubRoot pretends the process runs as root so preflightRoot lets the call
// through; tests still execute as the invoking developer's UID.
func stubRoot(t *testing.T) {
	t.Helper()
	prev := euid
	euid = func() int { return 0 }
	t.Cleanup(func() { euid = prev })
}

// stubChown defangs chownWorkingDir — we never want a test to chown a tmp
// directory to a real UID/GID on the developer machine.
func stubChown(t *testing.T) {
	t.Helper()
	prev := chownWorkingDir
	chownWorkingDir = func(InstallOptions) error { return nil }
	t.Cleanup(func() { chownWorkingDir = prev })
}

// stubUnitDir redirects the system unit file into a tempdir so tests don't
// require write access to /etc/systemd/system.
func stubUnitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := systemdUnitDir
	systemdUnitDir = dir
	t.Cleanup(func() { systemdUnitDir = prev })
}

func TestRenderSystemdUnit(t *testing.T) {
	got, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/home/me/.openbee/config.yaml",
		LogPath:    "/home/me/.openbee/daemon.log",
		WorkingDir: "/home/me/.openbee",
		Home:       "/home/me",
		EnvPath:    "/home/me/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin",
		RunAsUser:  "me",
		RunAsGroup: "me",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ExecStart=/usr/local/bin/openbee server -c /home/me/.openbee/config.yaml",
		"WorkingDirectory=/home/me/.openbee",
		"Restart=on-failure",
		"RestartSec=10",
		"StandardOutput=append:/home/me/.openbee/daemon.log",
		"WantedBy=multi-user.target",
		"After=network-online.target",
		"Environment=PATH=/home/me/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin",
		"User=me",
		"Group=me",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestLinuxInstall_WritesSystemUnit(t *testing.T) {
	stubRoot(t)
	stubChown(t)
	tmp := t.TempDir()
	stubUnitDir(t, filepath.Join(tmp, "systemd"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	var seen [][]string
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		seen = append(seen, args)
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	if err := mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
		WorkingDir: tmp,
		EnvPath:    "/opt/homebrew/bin:/usr/bin:/bin",
		Home:       tmp,
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
		AutoStart:  false,
	}); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(tmp, "systemd", "openbee.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(data), cfg) {
		t.Errorf("unit missing config path")
	}
	if !strings.Contains(string(data), "User=openbee") {
		t.Errorf("unit missing User= directive")
	}
	// Sanity check that we never passed --user to systemctl.
	for _, args := range seen {
		for _, a := range args {
			if a == "--user" {
				t.Errorf("unexpected --user in systemctl call: %v", args)
			}
		}
	}
}

func TestLinuxInstall_RefusesWithoutRoot(t *testing.T) {
	prevEuid := euid
	euid = func() int { return 1000 }
	t.Cleanup(func() { euid = prevEuid })

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	t.Cleanup(func() { execLookPath = prevLook })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/tmp/config.yaml",
		LogPath:    "/tmp/daemon.log",
		WorkingDir: "/tmp",
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
	})
	if err == nil {
		t.Fatal("expected refusal when not root")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("expected root-required error, got %v", err)
	}
}

func TestLinuxInstall_RollbackOnDaemonReloadFailure(t *testing.T) {
	stubRoot(t)
	stubChown(t)
	tmp := t.TempDir()
	stubUnitDir(t, filepath.Join(tmp, "systemd"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "daemon-reload" {
			return []byte("boom"), errors.New("exit status 1")
		}
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: filepath.Join(tmp, "config.yaml"),
		LogPath:    filepath.Join(tmp, "daemon.log"),
		WorkingDir: tmp,
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
	})
	if err == nil {
		t.Fatal("expected error when daemon-reload fails")
	}
	unitPath := filepath.Join(tmp, "systemd", "openbee.service")
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("unit file should be rolled back; stat err = %v", err)
	}
}

func TestLinuxInstall_ForcePreservesUnitOnFailure(t *testing.T) {
	stubRoot(t)
	stubChown(t)
	tmp := t.TempDir()
	unitDir := filepath.Join(tmp, "systemd")
	stubUnitDir(t, unitDir)
	unitPath := filepath.Join(unitDir, "openbee.service")
	if err := os.WriteFile(unitPath, []byte("# preexisting\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "daemon-reload" {
			return []byte("boom"), errors.New("exit status 1")
		}
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: filepath.Join(tmp, "config.yaml"),
		LogPath:    filepath.Join(tmp, "daemon.log"),
		WorkingDir: tmp,
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
		Force:      true,
	})
	if err == nil {
		t.Fatal("expected error when daemon-reload fails")
	}
	// With Force, the user accepted overwriting; we should not delete the new
	// unit (which would otherwise be more surprising than the failure itself).
	if _, err := os.Stat(unitPath); err != nil {
		t.Errorf("unit file should remain after force-overwrite failure; got %v", err)
	}
}
