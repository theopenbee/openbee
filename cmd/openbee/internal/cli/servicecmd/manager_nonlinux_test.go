//go:build !linux

package servicecmd

import (
	"os"
	"path/filepath"
	"testing"
)

// On non-Linux builds resolveRunAs returns an empty RunAsUser, so
// appendNodeWarning falls through to the execLookPath("node") probe. This test
// pins the fallback: when node is missing from the installer's PATH, the
// resolver must emit a warning. (Linux exercises the same warning via the
// stub-friendly runuser-based path in TestResolveInstallOptions_NotExecutableWarning.)
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

	_, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning when node is missing from PATH")
	}
}
