//go:build darwin

package servicecmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinStatus_ParsesLastExitInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	plistDir := filepath.Join(tmp, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plistDir, launchdLabel+".plist"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	const sample = `com.theopenbee.openbee = {
	active count = 0
	path = /Users/me/Library/LaunchAgents/com.theopenbee.openbee.plist
	state = not running
	last exit code = 78
	last exit reason = killed by signal: 9
}
`
	prevRun := runCommand
	runCommand = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(sample), nil
	}
	t.Cleanup(func() { runCommand = prevRun })

	st, err := (darwinManager{}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Errorf("Installed = false, want true")
	}
	if st.RunState != RunStateStopped {
		t.Errorf("RunState = %v, want stopped", st.RunState)
	}
	if st.LastExitCode != "78" {
		t.Errorf("LastExitCode = %q, want %q", st.LastExitCode, "78")
	}
	if st.LastExitReason != "killed by signal: 9" {
		t.Errorf("LastExitReason = %q", st.LastExitReason)
	}
}

func TestRenderLaunchdPlist(t *testing.T) {
	got, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/Users/me/.openbee/config.yaml",
		LogPath:    "/Users/me/.openbee/daemon.log",
		WorkingDir: "/Users/me/.openbee",
		Home:       "/Users/me",
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
		"<key>WorkingDirectory</key>",
		"<string>/Users/me/.openbee</string>",
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
