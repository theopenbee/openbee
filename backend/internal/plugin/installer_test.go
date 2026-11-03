package plugin

import (
	"context"
	"encoding/json"
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

func (f *fakeNpm) run(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
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
	return &Installer{Registry: OpenRegistry(dir), Runtime: nodeOnPath(t), Run: npm.run}, dir
}

func nodeOnPath(t *testing.T) *RuntimeResolver {
	return &RuntimeResolver{Dir: t.TempDir(), LookPath: onPath("node", "npm")}
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
		writePlugin(t, src, strings.Replace(validManifest, `"discord"`, `"local"`, 1))
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

func TestExecRunner_ReturnsStdoutOnly(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := ExecRunner(context.Background(), t.TempDir(), nil, "sh", "-c", "echo out; echo warn 1>&2")
	require.NoError(t, err)
	assert.Equal(t, "out\n", string(out))

	out, err = ExecRunner(context.Background(), t.TempDir(), []string{"OPENBEE_TEST_ENV=yes"}, "sh", "-c", "echo $OPENBEE_TEST_ENV")
	require.NoError(t, err)
	assert.Equal(t, "yes\n", string(out))

	_, err = ExecRunner(context.Background(), t.TempDir(), nil, "sh", "-c", "echo boom 1>&2; exit 3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

type upgradeNpm struct {
	fakeNpm
}

func (f *upgradeNpm) run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	if len(args) > 2 && args[0] == "install" && strings.HasSuffix(args[2], "@0.0.9") {
		f.calls = append(f.calls, name+" "+strings.Join(args, " "))
		pkgDir := filepath.Join(dir, "node_modules", "@x", "openbee-discord")
		return nil, os.WriteFile(filepath.Join(pkgDir, ManifestFile), []byte(validManifest), 0o644)
	}
	return f.fakeNpm.run(ctx, dir, env, name, args...)
}

func TestInstaller_FailedUpgradeRestoresPreviousVersion(t *testing.T) {
	npm := &upgradeNpm{fakeNpm{packJSON: `[{"name":"@x/openbee-discord"}]`, manifest: `{"protocol_version": 9}`}}
	dir := filepath.Join(t.TempDir(), "plugins")
	inst := &Installer{Registry: OpenRegistry(dir), Runtime: nodeOnPath(t), Run: npm.run}
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

type fakeBun struct {
	calls    []string
	env      []string
	value    string
	manifest string
}

func (f *fakeBun) run(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, filepath.Base(name)+" "+strings.Join(args, " "))
	f.env = env
	deps, err := dependencies(dir)
	if err != nil {
		return nil, err
	}
	if deps == nil {
		deps = map[string]string{}
	}
	switch args[0] {
	case "add":
		deps["@x/openbee-discord"] = f.value
		pkgDir := filepath.Join(dir, "node_modules", "@x", "openbee-discord")
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(pkgDir, ManifestFile), []byte(f.manifest), 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"@x/openbee-discord","version":"0.1.0"}`), 0o644); err != nil {
			return nil, err
		}
	case "remove":
		delete(deps, args[1])
	}
	data, err := json.Marshal(map[string]any{"name": "openbee-plugins", "private": true, "dependencies": deps})
	if err != nil {
		return nil, err
	}
	return nil, os.WriteFile(filepath.Join(dir, "package.json"), data, 0o644)
}

func newBunInstaller(t *testing.T, bun *fakeBun) (*Installer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "plugins")
	rt := &RuntimeResolver{Dir: t.TempDir(), LookPath: onPath("bun", "node", "npm")}
	return &Installer{Registry: OpenRegistry(dir), Runtime: rt, Run: bun.run}, dir
}

func TestInstaller_Bun(t *testing.T) {
	bun := &fakeBun{value: "^0.1.0", manifest: validManifest}
	inst, dir := newBunInstaller(t, bun)

	entry, _, err := inst.Install(context.Background(), "@x/openbee-discord@^0.1.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"bun add @x/openbee-discord@^0.1.0"}, bun.calls)
	assert.Equal(t, Entry{
		ID:      "discord",
		Source:  SourceNpm,
		Package: "@x/openbee-discord",
		Path:    filepath.Join(dir, "node_modules", "@x", "openbee-discord"),
		Version: "0.1.0",
	}, entry)

	_, _, err = inst.Install(context.Background(), "@x/openbee-discord")
	require.NoError(t, err, "reinstall leaves package.json unchanged")

	_, err = inst.Remove(context.Background(), "discord")
	require.NoError(t, err)
	assert.Equal(t, "bun remove @x/openbee-discord", bun.calls[len(bun.calls)-1])
	deps, err := dependencies(dir)
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func TestInstaller_BunGitSpecReinstall(t *testing.T) {
	bun := &fakeBun{value: "github:x/openbee-discord", manifest: validManifest}
	inst, _ := newBunInstaller(t, bun)
	for range 2 {
		entry, _, err := inst.Install(context.Background(), "github:x/openbee-discord")
		require.NoError(t, err)
		assert.Equal(t, "@x/openbee-discord", entry.Package)
	}
}

func TestInstaller_BunInvalidManifestRollsBack(t *testing.T) {
	bun := &fakeBun{value: "^0.1.0", manifest: `{"protocol_version": 9}`}
	inst, _ := newBunInstaller(t, bun)

	_, _, err := inst.Install(context.Background(), "@x/openbee-discord")
	require.Error(t, err)
	assert.Equal(t, []string{"bun add @x/openbee-discord", "bun remove @x/openbee-discord"}, bun.calls)
}

func TestInstaller_LocalBunGetsPrivateInstallDir(t *testing.T) {
	bun := &fakeBun{value: "^0.1.0", manifest: validManifest}
	inst, _ := newBunInstaller(t, bun)
	inst.Runtime.LookPath = onPath()
	writeLocalBun(t, inst.Runtime)

	_, _, err := inst.Install(context.Background(), "@x/openbee-discord")
	require.NoError(t, err)
	assert.Equal(t, localBunRuntime(inst.Runtime).Env, bun.env)
}

func TestAddedPackage(t *testing.T) {
	cases := []struct {
		name          string
		before, after map[string]string
		spec          string
		want          string
	}{
		{"new dependency", map[string]string{"a": "^1.0.0"}, map[string]string{"a": "^1.0.0", "@x/b": "^0.1.0"}, "@x/b", "@x/b"},
		{"changed range", map[string]string{"a": "^1.0.0"}, map[string]string{"a": "^1.1.0"}, "a@latest", "a"},
		{"unchanged registry spec", map[string]string{"@x/b": "^0.1.0"}, map[string]string{"@x/b": "^0.1.0"}, "@x/b@^0.1.0", "@x/b"},
		{"unchanged git spec", map[string]string{"@x/b": "github:x/b"}, map[string]string{"@x/b": "github:x/b"}, "github:x/b", "@x/b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := addedPackage(tc.before, tc.after, tc.spec)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := addedPackage(map[string]string{"a": "1"}, map[string]string{"a": "1"}, "./a.tgz")
	assert.Error(t, err)
	_, err = addedPackage(map[string]string{"a": "1"}, map[string]string{"a": "1"}, "b")
	assert.Error(t, err, "registry name must be a dependency")
}
