//go:build !linux

package servicecmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// On non-Linux builds resolveRunAs returns an empty RunAsUser, so
// appendNodeWarning falls through to the execLookPath("node") probe. This test
// pins the fallback: when node is missing from the installer's PATH, the
// resolver must emit a warning. (Linux exercises the same warning via the
// stub-friendly runuser-based path in TestResolveInstallOptions_NotExecutableWarning.)
func TestResolveInstallOptions_NodeMissingEmitsWarning(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))

	prev := execLookPath
	execLookPath = func(name string) (string, error) {
		if name == "node" {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + name, nil
	}
	t.Cleanup(func() { execLookPath = prev })

	_, warnings, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	require.NoError(t, err)
	require.NotEmpty(t, warnings, "expected a warning when node is missing from PATH")
}
