//go:build linux

package servicecmd

import (
	"context"
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
}
