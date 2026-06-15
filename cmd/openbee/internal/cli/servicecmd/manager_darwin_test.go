//go:build darwin

package servicecmd

import (
	"context"
	"errors"
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
		EnvPath:    "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin",
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
		"<key>PATH</key>",
		"<string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered plist missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestDarwinStop_UnloadsViaBootout(t *testing.T) {
	var got []string
	prevRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	}
	t.Cleanup(func() { runCommand = prevRun })

	if err := (darwinManager{}).Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"launchctl", "bootout", guiTarget() + "/" + launchdLabel}
	if !equalStrings(got, want) {
		t.Errorf("Stop invoked %v, want %v", got, want)
	}
}

func TestDarwinStart_KickstartsWhenLoaded(t *testing.T) {
	var calls [][]string
	prevRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil // print succeeds => loaded
	}
	t.Cleanup(func() { runCommand = prevRun })

	if err := (darwinManager{}).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2 (print + kickstart)", calls)
	}
	wantKickstart := []string{"launchctl", "kickstart", guiTarget() + "/" + launchdLabel}
	if !equalStrings(calls[1], wantKickstart) {
		t.Errorf("second call = %v, want %v", calls[1], wantKickstart)
	}
}

func TestDarwinStart_BootstrapsWhenNotLoaded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var calls [][]string
	prevRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(args) > 0 && args[0] == "print" {
			return nil, errors.New("not loaded")
		}
		return nil, nil
	}
	t.Cleanup(func() { runCommand = prevRun })

	if err := (darwinManager{}).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2 (print + bootstrap)", calls)
	}
	plistPath := filepath.Join(tmp, "Library", "LaunchAgents", launchdLabel+".plist")
	wantBootstrap := []string{"launchctl", "bootstrap", guiTarget(), plistPath}
	if !equalStrings(calls[1], wantBootstrap) {
		t.Errorf("second call = %v, want %v", calls[1], wantBootstrap)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
		EnvPath:    "/opt/homebrew/bin:/usr/bin:/bin",
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
	if !strings.Contains(string(data), "/opt/homebrew/bin:/usr/bin:/bin") {
		t.Errorf("plist missing PATH from EnvPath")
	}
}
