//go:build darwin

package servicecmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderLaunchdPlist(t *testing.T) {
	got, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/Users/me/.openbee/config.yaml",
		LogPath:    "/Users/me/.openbee/daemon.log",
		Home:       "/Users/me",
		Path:       "/usr/local/bin:/usr/bin:/bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>com.theopenbee.openbee</string>",
		"<string>/usr/local/bin/openbee</string>",
		"<string>server</string>",
		"<string>-c</string>",
		"<string>/Users/me/.openbee/config.yaml</string>",
		"<key>KeepAlive</key>",
		"<integer>10</integer>",
		"<string>/Users/me/.openbee/daemon.log</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered plist missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestDarwinInstall_WritesPlist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	mgr := darwinManager{}
	prev := execLookPath
	execLookPath = func(_ string) (string, error) { return "/usr/bin/launchctl", nil }
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }
	t.Cleanup(func() { execLookPath = prev; runCommand = prevRun })

	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)
	log := filepath.Join(tmp, "daemon.log")

	if err := mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    log,
		AutoStart:  false,
	}); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(tmp, "Library", "LaunchAgents", "com.theopenbee.openbee.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(data), cfg) {
		t.Errorf("plist missing config path")
	}
}
