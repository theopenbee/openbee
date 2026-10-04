package servicecmd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// currentUsername returns the user the test process runs as. Used to populate
// --run-as on Linux where resolveInstallOptions refuses to default.
func currentUsername(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	require.NoError(t, err)
	return u.Username
}

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

func TestResolveInstallOptions_ExplicitConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	// On Linux these probes shell out to runuser, whose login-shell PATH never
	// matches the test process PATH. ("", nil) means "no run-as PATH", so
	// EnvPath falls back to the installer PATH on every platform.
	stubLookupRunAsEnvPath(t, func(context.Context, string) (string, error) { return "", nil })
	stubVerifyNode(t, func(context.Context, string, string) nodeCheckResult { return nodeCheckOK })

	opts, _, err := resolveInstallOptions(cfg, "", currentUsername(t), false, false)
	require.NoError(t, err)
	assert.Equal(t, cfg, opts.ConfigPath)
	assert.True(t, opts.AutoStart, "AutoStart should default to true")
	assert.NotEmpty(t, opts.ExePath)
	assert.Equal(t, filepath.Join(testHome, ".openbee", "openbee.log"), opts.LogPath)
	assert.Equal(t, filepath.Join(testHome, ".openbee"), opts.WorkingDir, "WorkingDir should default to <home>/.openbee")
	assert.NotEmpty(t, opts.EnvPath, "EnvPath should capture the install-time PATH")
	assert.Equal(t, os.Getenv("PATH"), opts.EnvPath)
}

func TestResolveInstallOptions_MissingConfig(t *testing.T) {
	_, _, err := resolveInstallOptions("/nonexistent/path.yaml", "", currentUsername(t), false, false)
	require.Error(t, err, "expected error for missing config")
}

// Regression: passing a directory as --config (e.g. `--config ~/openbee`)
// previously silently proceeded because os.Stat treats dirs as existing files.
func TestResolveInstallOptions_ConfigIsDirectory(t *testing.T) {
	tmp := t.TempDir()
	_, _, err := resolveInstallOptions(tmp, "", currentUsername(t), false, false)
	require.Error(t, err, "expected error for directory passed as config")
	assert.Contains(t, err.Error(), "must point to a config file")
	suggested := filepath.Join(tmp, "config.yaml")
	assert.Contains(t, err.Error(), suggested)
}

func TestResolveInstallOptions_ExplicitWorkingDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("{}"), 0o600))
	wd := filepath.Join(tmp, "run")

	opts, _, err := resolveInstallOptions(cfg, wd, currentUsername(t), false, false)
	require.NoError(t, err)
	assert.Equal(t, wd, opts.WorkingDir)
	_, err = os.Stat(wd)
	assert.NoError(t, err, "working dir not created")
}
