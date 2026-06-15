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

	opts, err := resolveInstallOptions(cfg, "", false, false)
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
}

func TestResolveInstallOptions_MissingConfig(t *testing.T) {
	_, err := resolveInstallOptions("/nonexistent/path.yaml", "", false, false)
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

	opts, err := resolveInstallOptions(cfg, wd, false, false)
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
