package servicecmd

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// currentUsername returns the user the test process runs as. Used to populate
// --run-as on Linux where resolveInstallOptions refuses to default.
func currentUsername(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	return u.Username
}

func TestResolveInstallOptions_ExplicitConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, _, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if opts.ConfigPath != cfg {
		t.Errorf("ConfigPath = %q, want %q", opts.ConfigPath, cfg)
	}
	if opts.AutoStart != true {
		t.Errorf("AutoStart should default to true")
	}
	if opts.ExePath == "" {
		t.Errorf("ExePath empty")
	}
	if opts.LogPath == "" {
		t.Errorf("LogPath empty")
	}
	if opts.WorkingDir == "" {
		t.Errorf("WorkingDir empty")
	}
	if !filepath.IsAbs(opts.WorkingDir) {
		t.Errorf("WorkingDir %q must be absolute", opts.WorkingDir)
	}
	if opts.EnvPath == "" {
		t.Errorf("EnvPath should capture the install-time PATH, got empty")
	}
	if opts.EnvPath != os.Getenv("PATH") {
		t.Errorf("EnvPath = %q, want current PATH %q", opts.EnvPath, os.Getenv("PATH"))
	}
}

func TestResolveInstallOptions_MissingConfig(t *testing.T) {
	_, _, err := resolveInstallOptions("/nonexistent/path.yaml", "", currentUsername(t), false, false)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

// Regression: passing a directory as --config (e.g. `--config ~/openbee`)
// previously silently proceeded because os.Stat treats dirs as existing files.
func TestResolveInstallOptions_ConfigIsDirectory(t *testing.T) {
	tmp := t.TempDir()
	_, _, err := resolveInstallOptions(tmp, "", currentUsername(t), false, false)
	if err == nil {
		t.Fatal("expected error for directory passed as config")
	}
	if !strings.Contains(err.Error(), "must point to a config file") {
		t.Errorf("expected directory-not-file error, got: %v", err)
	}
	suggested := filepath.Join(tmp, "config.yaml")
	if !strings.Contains(err.Error(), suggested) {
		t.Errorf("error should suggest %q, got: %v", suggested, err)
	}
}

func TestResolveInstallOptions_ExplicitWorkingDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd := filepath.Join(tmp, "run")

	opts, _, err := resolveInstallOptions(cfg, wd, currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if opts.WorkingDir != wd {
		t.Errorf("WorkingDir = %q, want %q", opts.WorkingDir, wd)
	}
	if _, err := os.Stat(wd); err != nil {
		t.Errorf("working dir not created: %v", err)
	}
}
