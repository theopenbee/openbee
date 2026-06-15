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

func TestRenderSystemdUnit(t *testing.T) {
	got, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/home/me/.openbee/config.yaml",
		LogPath:    "/home/me/.openbee/daemon.log",
		WorkingDir: "/home/me/.openbee",
		Home:       "/home/me",
		EnvPath:    "/home/me/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin",
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
		"WantedBy=default.target",
		"Environment=PATH=/home/me/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestLinuxInstall_WritesUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }
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
		EnvPath:    "/opt/homebrew/bin:/usr/bin:/bin",
		AutoStart:  false,
	}); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(tmp, ".config", "systemd", "user", "openbee.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(data), cfg) {
		t.Errorf("unit missing config path")
	}
	if !strings.Contains(string(data), "Environment=PATH=/opt/homebrew/bin:/usr/bin:/bin") {
		t.Errorf("unit missing PATH from EnvPath")
	}
}

// busPermDeniedOut is the exact stderr systemd prints when the user has no
// reachable D-Bus session (e.g. SSH without linger). The runtime wraps the
// CombinedOutput into the returned error message via runOrWrap, so the
// permission-denied detector has to match against both the output bytes and
// the error string.
const busPermDeniedOut = "Failed to connect to bus: Permission denied"

func TestLinuxInstall_RollbackOnDaemonReloadFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "--user" && args[1] == "show-environment" {
			return []byte("PATH=/usr/bin\n"), nil
		}
		if len(args) >= 2 && args[0] == "--user" && args[1] == "daemon-reload" {
			return []byte(busPermDeniedOut), errors.New("exit status 1")
		}
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
	})
	if err == nil {
		t.Fatal("expected error when daemon-reload fails")
	}
	if !strings.Contains(err.Error(), "systemd user bus") {
		t.Errorf("error should be the friendly user-bus message, got: %v", err)
	}
	unitPath := filepath.Join(tmp, ".config", "systemd", "user", "openbee.service")
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("unit file should be rolled back; stat err = %v", err)
	}
}

func TestLinuxInstall_PreflightBusFailureKeepsNoUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "--user" && args[1] == "show-environment" {
			return []byte(busPermDeniedOut), errors.New("exit status 1")
		}
		t.Fatalf("unexpected command after preflight failure: %v", args)
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
	})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	if !strings.Contains(err.Error(), "systemd user bus") {
		t.Errorf("error should be the friendly user-bus message, got: %v", err)
	}
	unitPath := filepath.Join(tmp, ".config", "systemd", "user", "openbee.service")
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("unit file should not exist when preflight fails; stat err = %v", err)
	}
}

func TestLinuxInstall_ForcePreservesUnitOnFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	unitDir := filepath.Join(tmp, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(unitDir, "openbee.service")
	if err := os.WriteFile(unitPath, []byte("# preexisting\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prevLook := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/systemctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "--user" && args[1] == "show-environment" {
			return []byte("PATH=/usr/bin\n"), nil
		}
		if len(args) >= 2 && args[0] == "--user" && args[1] == "daemon-reload" {
			return []byte(busPermDeniedOut), errors.New("exit status 1")
		}
		return nil, nil
	}
	t.Cleanup(func() { execLookPath = prevLook; runCommand = prevRun })

	mgr, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
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
