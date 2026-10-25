package plugin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNpm struct {
	calls    []string
	packJSON string
	manifest string
	err      error
}

func (f *fakeNpm) run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.err != nil {
		return nil, f.err
	}
	if len(args) > 0 && args[0] == "pack" {
		return []byte(f.packJSON), nil
	}
	if len(args) > 0 && args[0] == "install" {
		pkgDir := filepath.Join(dir, "node_modules", "@x", "openbee-discord")
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(pkgDir, ManifestFile), []byte(f.manifest), 0o644); err != nil {
			return nil, err
		}
		return nil, os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"@x/openbee-discord","version":"0.1.0"}`), 0o644)
	}
	return nil, nil
}

func newTestInstaller(t *testing.T, npm *fakeNpm) (*Installer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "plugins")
	return &Installer{Registry: OpenRegistry(dir), Run: npm.run}, dir
}

func TestInstaller_LocalPath(t *testing.T) {
	npm := &fakeNpm{}
	inst, _ := newTestInstaller(t, npm)
	src := t.TempDir()
	writePlugin(t, src, validManifest)

	entry, m, err := inst.Install(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, "discord", m.ID)
	abs, _ := filepath.Abs(src)
	assert.Equal(t, Entry{ID: "discord", Source: SourcePath, Path: abs}, entry)
	assert.Empty(t, npm.calls)

	got, ok, err := inst.Registry.Get("discord")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestInstaller_Npm(t *testing.T) {
	npm := &fakeNpm{packJSON: `[{"name":"@x/openbee-discord","version":"0.1.0"}]`, manifest: validManifest}
	inst, dir := newTestInstaller(t, npm)

	entry, m, err := inst.Install(context.Background(), "@x/openbee-discord@0.1.0")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"npm pack --dry-run --json @x/openbee-discord@0.1.0",
		"npm install --save @x/openbee-discord@0.1.0",
	}, npm.calls)
	assert.Equal(t, "discord", m.ID)
	assert.Equal(t, Entry{
		ID:      "discord",
		Source:  SourceNpm,
		Package: "@x/openbee-discord",
		Path:    filepath.Join(dir, "node_modules", "@x", "openbee-discord"),
		Version: "0.1.0",
	}, entry)

	pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
	require.NoError(t, err)
	assert.Contains(t, string(pkg), `"private": true`)
}

func TestInstaller_NpmInvalidManifestRollsBack(t *testing.T) {
	npm := &fakeNpm{packJSON: `[{"name":"@x/openbee-discord"}]`, manifest: `{"protocol_version": 9}`}
	inst, _ := newTestInstaller(t, npm)

	_, _, err := inst.Install(context.Background(), "@x/openbee-discord")
	require.Error(t, err)
	assert.Equal(t, "npm uninstall --save @x/openbee-discord", npm.calls[len(npm.calls)-1])
	list, err := inst.Registry.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestInstaller_RejectsConflicts(t *testing.T) {
	t.Run("reserved id", func(t *testing.T) {
		inst, _ := newTestInstaller(t, &fakeNpm{})
		src := t.TempDir()
		writePlugin(t, src, strings.Replace(validManifest, `"discord"`, `"weixin"`, 1))
		_, _, err := inst.Install(context.Background(), src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
	})
	t.Run("id taken by another path", func(t *testing.T) {
		inst, _ := newTestInstaller(t, &fakeNpm{})
		require.NoError(t, inst.Registry.Put(Entry{ID: "discord", Source: SourcePath, Path: "/elsewhere"}))
		src := t.TempDir()
		writePlugin(t, src, validManifest)
		_, _, err := inst.Install(context.Background(), src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already installed")
	})
	t.Run("same path reinstall upgrades", func(t *testing.T) {
		inst, _ := newTestInstaller(t, &fakeNpm{})
		src := t.TempDir()
		writePlugin(t, src, validManifest)
		_, _, err := inst.Install(context.Background(), src)
		require.NoError(t, err)
		_, _, err = inst.Install(context.Background(), src)
		require.NoError(t, err)
	})
}

func TestInstaller_RemoveNpm(t *testing.T) {
	npm := &fakeNpm{}
	inst, _ := newTestInstaller(t, npm)
	require.NoError(t, inst.Registry.Put(Entry{ID: "discord", Source: SourceNpm, Package: "@x/openbee-discord", Path: "/p"}))

	entry, err := inst.Remove(context.Background(), "discord")
	require.NoError(t, err)
	assert.Equal(t, "discord", entry.ID)
	assert.Equal(t, []string{"npm uninstall --save @x/openbee-discord"}, npm.calls)
	_, ok, err := inst.Registry.Get("discord")
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = inst.Remove(context.Background(), "nope")
	assert.True(t, errors.Is(err, ErrNotInstalled))
}

func TestInstaller_RemovePathDoesNotCallNpm(t *testing.T) {
	npm := &fakeNpm{}
	inst, _ := newTestInstaller(t, npm)
	require.NoError(t, inst.Registry.Put(Entry{ID: "discord", Source: SourcePath, Path: "/p"}))
	_, err := inst.Remove(context.Background(), "discord")
	require.NoError(t, err)
	assert.Empty(t, npm.calls)
}

func TestInstaller_NpmMissing(t *testing.T) {
	inst, _ := newTestInstaller(t, &fakeNpm{err: &exec.Error{Name: "npm", Err: exec.ErrNotFound}})
	_, _, err := inst.Install(context.Background(), "@x/openbee-discord")
	assert.True(t, errors.Is(err, ErrNpmNotFound), "got %v", err)
}

func TestExecRunner_ReturnsStdoutOnly(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := ExecRunner(context.Background(), t.TempDir(), "sh", "-c", "echo out; echo warn 1>&2")
	require.NoError(t, err)
	assert.Equal(t, "out\n", string(out))

	_, err = ExecRunner(context.Background(), t.TempDir(), "sh", "-c", "echo boom 1>&2; exit 3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

type upgradeNpm struct {
	fakeNpm
}

func (f *upgradeNpm) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if len(args) > 2 && args[0] == "install" && strings.HasSuffix(args[2], "@0.0.9") {
		f.calls = append(f.calls, name+" "+strings.Join(args, " "))
		pkgDir := filepath.Join(dir, "node_modules", "@x", "openbee-discord")
		return nil, os.WriteFile(filepath.Join(pkgDir, ManifestFile), []byte(validManifest), 0o644)
	}
	return f.fakeNpm.run(ctx, dir, name, args...)
}

func TestInstaller_FailedUpgradeRestoresPreviousVersion(t *testing.T) {
	npm := &upgradeNpm{fakeNpm{packJSON: `[{"name":"@x/openbee-discord"}]`, manifest: `{"protocol_version": 9}`}}
	dir := filepath.Join(t.TempDir(), "plugins")
	inst := &Installer{Registry: OpenRegistry(dir), Run: npm.run}
	previous := Entry{ID: "discord", Source: SourceNpm, Package: "@x/openbee-discord", Path: filepath.Join(dir, "node_modules", "@x", "openbee-discord"), Version: "0.0.9"}
	require.NoError(t, inst.Registry.Put(previous))

	_, _, err := inst.Install(context.Background(), "@x/openbee-discord@latest")
	require.Error(t, err)
	assert.Equal(t, "npm install --save @x/openbee-discord@0.0.9", npm.calls[len(npm.calls)-1])
	for _, c := range npm.calls {
		assert.NotContains(t, c, "uninstall")
	}
	got, ok, err := inst.Registry.Get("discord")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, previous, got)
	_, err = LoadManifest(previous.Path)
	assert.NoError(t, err)
}
