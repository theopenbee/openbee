//go:build darwin

package servicecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDarwinStatus_ParsesLastExitInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	plistDir := filepath.Join(tmp, "Library", "LaunchAgents")
	require.NoError(t, os.MkdirAll(plistDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plistDir, launchdLabel+".plist"), []byte("ignored"), 0o644))

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
	require.NoError(t, err)
	assert.True(t, st.Installed, "Installed = false, want true")
	assert.Equal(t, RunStateStopped, st.RunState)
	assert.Equal(t, "78", st.LastExitCode)
	assert.Equal(t, "killed by signal: 9", st.LastExitReason)
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
	require.NoError(t, err)
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
		assert.Contains(t, got, want)
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

	require.NoError(t, (darwinManager{}).Stop(context.Background()))
	want := []string{"launchctl", "bootout", launchdTarget()}
	assert.Equal(t, want, got)
}

func TestDarwinStart_KickstartsWhenLoaded(t *testing.T) {
	var calls [][]string
	prevRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil // print succeeds => loaded
	}
	t.Cleanup(func() { runCommand = prevRun })

	require.NoError(t, (darwinManager{}).Start(context.Background()))
	require.Len(t, calls, 2, "calls = %v, want 2 (print + kickstart)", calls)
	wantKickstart := []string{"launchctl", "kickstart", launchdTarget()}
	assert.Equal(t, wantKickstart, calls[1])
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

	require.NoError(t, (darwinManager{}).Start(context.Background()))
	require.Len(t, calls, 2, "calls = %v, want 2 (print + bootstrap)", calls)
	plistPath := filepath.Join(tmp, "Library", "LaunchAgents", launchdLabel+".plist")
	wantBootstrap := []string{"launchctl", "bootstrap", guiTarget(), plistPath}
	assert.Equal(t, wantBootstrap, calls[1])
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

	err := mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    log,
		EnvPath:    "/opt/homebrew/bin:/usr/bin:/bin",
		AutoStart:  false,
	})
	require.NoError(t, err)
	// Content checks live in TestRenderLaunchdPlist; this test only confirms
	// the install path actually drops the plist under ~/Library/LaunchAgents.
	plistPath := filepath.Join(tmp, "Library", "LaunchAgents", "com.theopenbee.openbee.plist")
	_, err = os.Stat(plistPath)
	require.NoError(t, err, "plist not written")
}
