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

// stubLookupRunAsEnvPath swaps in a deterministic PATH resolver so tests can
// exercise the success / failure paths without depending on a real `runuser`.
func stubLookupRunAsEnvPath(t *testing.T, fn func(ctx context.Context, username string) (string, error)) {
	t.Helper()
	prev := lookupRunAsEnvPath
	lookupRunAsEnvPath = fn
	t.Cleanup(func() { lookupRunAsEnvPath = prev })
}

// stubVerifyNode swaps in a deterministic node-availability check so we can
// fire each warning branch (missing / not-executable / ok / unknown) without
// shelling out.
func stubVerifyNode(t *testing.T, fn func(ctx context.Context, username, envPath string) nodeCheckResult) {
	t.Helper()
	prev := verifyNodeForRunAsUser
	verifyNodeForRunAsUser = fn
	t.Cleanup(func() { verifyNodeForRunAsUser = prev })
}

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

// TestResolveInstallOptions_UsesRunAsUserPath is the core regression for the
// `/usr/bin/env: 'node': Permission denied` chat-time failure: we must embed
// the run-as user's PATH into the unit, not the installer's, otherwise sudo's
// secure_path or /root/.nvm leaks through and the daemon user can't exec node.
func TestResolveInstallOptions_UsesRunAsUserPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	const userPath = "/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin:/bin"
	stubLookupRunAsEnvPath(t, func(_ context.Context, _ string) (string, error) {
		return userPath, nil
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckOK
	})

	opts, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("resolveInstallOptions: %v", err)
	}
	if opts.EnvPath != userPath {
		t.Errorf("EnvPath = %q, want run-as user path %q", opts.EnvPath, userPath)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when lookup + verify succeed, got %v", warnings)
	}
}

func TestResolveInstallOptions_FallsBackOnLookupFailure(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubLookupRunAsEnvPath(t, func(context.Context, string) (string, error) {
		return "", errors.New("runuser missing")
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckOK
	})

	opts, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("resolveInstallOptions: %v", err)
	}
	if opts.EnvPath != os.Getenv("PATH") {
		t.Errorf("EnvPath = %q, want installer PATH fallback %q", opts.EnvPath, os.Getenv("PATH"))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "runuser missing") {
		t.Errorf("expected RunAsPathResolveFailed warning, got %v", warnings)
	}
}

func TestResolveInstallOptions_NotExecutableWarning(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubLookupRunAsEnvPath(t, func(context.Context, string) (string, error) {
		return "/root/.nvm/versions/node/v20.0.0/bin:/usr/bin", nil
	})
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult {
		return nodeCheckNotExecutable
	})

	_, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("resolveInstallOptions: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected NodeNotExecutableWarning")
	}
	// The warning must point at the Permission-denied fix path (per-user node),
	// not the "install Node.js" path that NodeMissingWarning suggests.
	if !strings.Contains(warnings[0], "Permission denied") && !strings.Contains(warnings[0], "无权执行") {
		t.Errorf("warning should mention Permission denied, got %q", warnings[0])
	}
}

// TestLinuxLookupRunAsEnvPath_ShellsOutToRunuser verifies the production helper
// invokes runuser with the expected argv and trims the PATH it prints.
func TestLinuxLookupRunAsEnvPath_ShellsOutToRunuser(t *testing.T) {
	prev := runCommand
	var got []string
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return []byte("/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin\n"), nil
	}
	t.Cleanup(func() { runCommand = prev })

	p, err := linuxLookupRunAsEnvPath(context.Background(), "openbee")
	if err != nil {
		t.Fatalf("linuxLookupRunAsEnvPath: %v", err)
	}
	want := "/home/openbee/.nvm/versions/node/v20.0.0/bin:/usr/bin"
	if p != want {
		t.Errorf("PATH = %q, want %q", p, want)
	}
	wantArgs := []string{"runuser", "-l", "openbee", "-c", `printf %s "$PATH"`}
	if strings.Join(got, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("runuser argv = %v, want %v", got, wantArgs)
	}
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
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
