package servicecmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInstallOptions_ExplicitConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, _, err := resolveInstallOptions(cfg, "", false, false)
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

func TestResolveInstallOptions_NodeMissingEmitsWarning(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	prev := execLookPath
	execLookPath = func(name string) (string, error) {
		if name == "node" {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + name, nil
	}
	t.Cleanup(func() { execLookPath = prev })

	_, warnings, err := resolveInstallOptions(cfg, "", false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning when node is missing from PATH")
	}
}

func TestResolveInstallOptions_MissingConfig(t *testing.T) {
	_, _, err := resolveInstallOptions("/nonexistent/path.yaml", "", false, false)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestResolveInstallOptions_ExplicitWorkingDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd := filepath.Join(tmp, "run")

	opts, _, err := resolveInstallOptions(cfg, wd, false, false)
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
