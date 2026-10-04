//go:build linux

package servicecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, os.MkdirAll(dir, 0o755))
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
		RunAsUser:  "me",
		RunAsGroup: "me",
	})
	require.NoError(t, err)
	for _, want := range []string{
		// ExecStart goes through `bash -ilc` so the daemon inherits the run-as
		// user's interactive-login PATH (nvm/conda sourced from ~/.bashrc as
		// well as ~/.bash_profile) at start time. `exec` avoids the extra bash
		// process so systemd still tracks the server PID.
		"ExecStart=/bin/bash -ilc 'exec /usr/local/bin/openbee server -c /home/me/.openbee/config.yaml'",
		"WorkingDirectory=/home/me/.openbee",
		"Restart=on-failure",
		"RestartSec=10",
		"StandardOutput=append:/home/me/.openbee/daemon.log",
		"WantedBy=multi-user.target",
		"After=network-online.target",
		"User=me",
		"Group=me",
	} {
		assert.Contains(t, got, want)
	}
	// PATH/HOME must NOT be frozen into the unit — that defeats the bash -ilc
	// design and brings back the install-time snapshot bugs.
	for _, forbidden := range []string{
		"Environment=PATH=",
		"Environment=HOME=",
	} {
		assert.NotContains(t, got, forbidden, "unit must not bake %q into the unit (use bash -lc instead)", forbidden)
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
	require.NoError(t, err)
	cfg := filepath.Join(tmp, "config.yaml")
	_ = os.WriteFile(cfg, []byte("{}"), 0o600)

	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: cfg,
		LogPath:    filepath.Join(tmp, "daemon.log"),
		WorkingDir: tmp,
		EnvPath:    "/opt/homebrew/bin:/usr/bin:/bin",
		Home:       tmp,
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
		AutoStart:  false,
	})
	require.NoError(t, err)
	unitPath := filepath.Join(tmp, "systemd", "openbee.service")
	data, err := os.ReadFile(unitPath)
	require.NoError(t, err, "unit not written")
	assert.Contains(t, string(data), cfg, "unit missing config path")
	assert.Contains(t, string(data), "User=openbee", "unit missing User= directive")
	// Sanity check that we never passed --user to systemctl.
	for _, args := range seen {
		for _, a := range args {
			assert.NotEqual(t, "--user", a, "unexpected --user in systemctl call: %v", args)
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
	require.NoError(t, err)
	err = mgr.Install(context.Background(), InstallOptions{
		ExePath:    "/usr/local/bin/openbee",
		ConfigPath: "/tmp/config.yaml",
		LogPath:    "/tmp/daemon.log",
		WorkingDir: "/tmp",
		RunAsUser:  "openbee",
		RunAsGroup: "openbee",
	})
	require.Error(t, err, "expected refusal when not root")
	assert.Contains(t, err.Error(), "root")
}

// TestLinuxInstall_FailureHandling merges the plain and --force variants of a
// daemon-reload failure during install: both inject a systemctl daemon-reload
// error and differ only in whether Force is set and, consequently, whether
// the just-written unit file is rolled back or left in place.
func TestLinuxInstall_FailureHandling(t *testing.T) {
	cases := []struct {
		name           string
		force          bool
		wantUnitExists bool
	}{
		{name: "RollbackOnDaemonReloadFailure", force: false, wantUnitExists: false},
		{name: "ForcePreservesUnitOnFailure", force: true, wantUnitExists: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubRoot(t)
			stubChown(t)
			tmp := t.TempDir()
			unitDir := filepath.Join(tmp, "systemd")
			stubUnitDir(t, unitDir)
			unitPath := filepath.Join(unitDir, "openbee.service")
			if tc.force {
				require.NoError(t, os.WriteFile(unitPath, []byte("# preexisting\n"), 0o644))
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
			require.NoError(t, err)
			err = mgr.Install(context.Background(), InstallOptions{
				ExePath:    "/usr/local/bin/openbee",
				ConfigPath: filepath.Join(tmp, "config.yaml"),
				LogPath:    filepath.Join(tmp, "daemon.log"),
				WorkingDir: tmp,
				RunAsUser:  "openbee",
				RunAsGroup: "openbee",
				Force:      tc.force,
			})
			require.Error(t, err, "expected error when daemon-reload fails")

			_, statErr := os.Stat(unitPath)
			if tc.wantUnitExists {
				// With Force, the user accepted overwriting; we should not delete
				// the new unit (which would otherwise be more surprising than the
				// failure itself).
				assert.NoError(t, statErr, "unit file should remain after force-overwrite failure")
			} else {
				assert.True(t, os.IsNotExist(statErr), "unit file should be rolled back; stat err = %v", statErr)
			}
		})
	}
}

// TestResolveInstallOptions_UsesRunAsUserPath is the core regression for the
// `/usr/bin/env: 'node': Permission denied` chat-time failure: we must embed
// the run-as user's PATH into the unit, not the installer's, otherwise sudo's
// secure_path or /root/.nvm leaks through and the daemon user can't exec node.
func TestResolveInstallOptions_UsesRunAsUserPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	const userPath = "/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin"
	stubLookupRunAsEnvPath(t, func(_ context.Context, _ string) (string, error) {
		return userPath, nil
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckOK
	})

	opts, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	require.NoError(t, err)
	assert.Equal(t, userPath, opts.EnvPath, "want run-as user path")
	assert.Empty(t, warnings, "expected no warnings when lookup + verify succeed")
}

func TestResolveInstallOptions_FallsBackOnLookupFailure(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	stubLookupRunAsEnvPath(t, func(context.Context, string) (string, error) {
		return "", errors.New("runuser missing")
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckOK
	})

	opts, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	require.NoError(t, err)
	assert.Equal(t, os.Getenv("PATH"), opts.EnvPath, "want installer PATH fallback")
	require.NotEmpty(t, warnings, "expected RunAsPathResolveFailed warning") // guard: next assertion indexes warnings[0]
	assert.Contains(t, warnings[0], "runuser missing")
}

func TestResolveInstallOptions_NotExecutableWarning(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	stubLookupRunAsEnvPath(t, func(context.Context, string) (string, error) {
		return "/root/.nvm/versions/node/v20.0.0/bin:/usr/bin", nil
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckNotExecutable
	})

	_, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	require.NoError(t, err)
	require.NotEmpty(t, warnings, "expected NodeNotExecutableWarning")
	// The warning must point at the Permission-denied fix path (per-user node),
	// not the "install Node.js" path that NodeMissingWarning suggests.
	assert.True(t, strings.Contains(warnings[0], "Permission denied") || strings.Contains(warnings[0], "无权执行"),
		"warning should mention Permission denied, got %q", warnings[0])
}

// TestLinuxLookupRunAsEnvPath_ShellsOutToRunuser verifies the production helper
// invokes runuser with the expected argv and extracts the marked PATH line
// from output polluted by shell chatter on stdout/stderr.
func TestLinuxLookupRunAsEnvPath_ShellsOutToRunuser(t *testing.T) {
	prev := runCommand
	var got []string
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return []byte("bash: cannot set terminal process group (1): Inappropriate ioctl for device\n" +
			"bash: no job control in this shell\n" +
			"Welcome from .bashrc\n" +
			runAsPathMarker + "/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin\n"), nil
	}
	t.Cleanup(func() { runCommand = prev })

	p, err := linuxLookupRunAsEnvPath(context.Background(), "openbee")
	require.NoError(t, err)
	want := "/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin"
	assert.Equal(t, want, p)
	wantArgs := []string{"runuser", "-l", "openbee", "-c", `bash -ic 'printf "__OPENBEE_PATH__=%s\n" "$PATH"'`}
	assert.Equal(t, wantArgs, got, "runuser argv")
}

func TestLinuxLookupRunAsEnvPath_MissingMarker(t *testing.T) {
	prev := runCommand
	runCommand = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("bash: no job control in this shell\n"), nil
	}
	t.Cleanup(func() { runCommand = prev })

	_, err := linuxLookupRunAsEnvPath(context.Background(), "openbee")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no PATH line")
}

func TestLinuxVerifyNodeForRunAsUser_MapsExitCodes(t *testing.T) {
	cases := []struct {
		name string
		code int
		err  error
		want nodeCheckResult
	}{
		{"executable", 0, nil, nodeCheckOK},
		{"missing", 1, nil, nodeCheckMissing},
		{"not_executable", 2, nil, nodeCheckNotExecutable},
		{"other_code", 99, nil, nodeCheckUnknown},
		{"runuser_missing", -1, errors.New("runuser: not found"), nodeCheckUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := runWithExitCode
			runWithExitCode = func(context.Context, string, ...string) (int, error) {
				return tc.code, tc.err
			}
			t.Cleanup(func() { runWithExitCode = prev })

			got := linuxVerifyNodeForRunAsUser(context.Background(), "openbee", "/usr/bin")
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestLinuxVerifyNodeForRunAsUser_UsesInteractiveLoginShell pins the argv
// shape: the probe must invoke `runuser -l <user> -c 'bash -ic <script>'` so
// it reads PATH from the same interactive-login shell the daemon will use at
// runtime. The earlier login-only form (no inner `bash -ic`) missed nvm setups
// that only patch PATH from ~/.bashrc, producing false-negative node warnings
// at install while the daemon still found node at runtime — and vice versa.
func TestLinuxVerifyNodeForRunAsUser_UsesInteractiveLoginShell(t *testing.T) {
	prev := runWithExitCode
	var got []string
	runWithExitCode = func(_ context.Context, name string, args ...string) (int, error) {
		got = append([]string{name}, args...)
		return 0, nil
	}
	t.Cleanup(func() { runWithExitCode = prev })

	// Exit-code → result mapping is covered by TestLinuxVerifyNodeForRunAsUser_MapsExitCodes;
	// this test only pins the argv shape.
	linuxVerifyNodeForRunAsUser(context.Background(), "openbee", "/ignored")
	require.GreaterOrEqual(t, len(got), 5, "expected `runuser -l openbee -c <script>`, got %v", got) // guard: indexes below read got[0..4]
	assert.Equal(t, []string{"runuser", "-l", "openbee", "-c"}, got[:4])
	assert.True(t, strings.HasPrefix(got[4], "bash -ic "), "expected inner shell to be `bash -ic …`, got %q", got[4])
}
