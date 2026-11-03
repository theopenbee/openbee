package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func onPath(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return name, nil
		}
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
}

func writeLocalBun(t *testing.T, r *RuntimeResolver) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(r.bunExe()), 0o755))
	require.NoError(t, os.WriteFile(r.bunExe(), []byte("#!/bin/sh\n"), 0o755))
}

type bunServer struct {
	*httptest.Server
	downloads atomic.Int32
}

const fakeBunScript = "#!/bin/sh\necho " + BunVersion + "\n"

func hostBunTarget(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake bun is a shell script")
	}
	target, err := bunTarget(runtime.GOOS, runtime.GOARCH, hasAVX2(), isMusl())
	if err != nil {
		t.Skip(err.Error())
	}
	return target
}

func newBunServer(t *testing.T, script string, corrupt bool) *bunServer {
	t.Helper()
	target := hostBunTarget(t)
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	fh := &zip.FileHeader{Name: "bun-" + target + "/bun", Method: zip.Deflate}
	fh.SetMode(0o755)
	w, err := zw.CreateHeader(fh)
	require.NoError(t, err)
	_, err = w.Write([]byte(script))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	sum := sha256.Sum256(archive.Bytes())
	if corrupt {
		sum[0] ^= 0xff
	}
	orig := bunChecksums[target]
	bunChecksums[target] = hex.EncodeToString(sum[:])
	t.Cleanup(func() { bunChecksums[target] = orig })
	path := "/bun-v" + BunVersion + "/bun-" + target + ".zip"
	s := &bunServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		s.downloads.Add(1)
		w.Write(archive.Bytes())
	}))
	t.Cleanup(s.Close)
	return s
}

func localBunRuntime(r *RuntimeResolver) Runtime {
	home := filepath.Join(r.Dir, "bun-"+BunVersion)
	return Runtime{Kind: RuntimeBun, Exe: filepath.Join(home, "bin", "bun"), Env: []string{
		"BUN_INSTALL=" + home,
		"BUN_INSTALL_CACHE_DIR=" + filepath.Join(home, "install", "cache"),
		"BUN_RUNTIME_TRANSPILER_CACHE_PATH=" + filepath.Join(home, "transpiler-cache"),
	}}
}

func TestResolve_Order(t *testing.T) {
	cases := []struct {
		name     string
		path     []string
		localBun bool
		want     Runtime
	}{
		{"bun on PATH wins", []string{"bun", "node", "npm"}, true, Runtime{Kind: RuntimeBun, Exe: "bun"}},
		{"local bun before node", []string{"node", "npm"}, true, Runtime{Kind: RuntimeBun}},
		{"node with npm", []string{"node", "npm"}, false, Runtime{Kind: RuntimeNode, Exe: "node", Npm: "npm"}},
		{"node without npm runs plugins", []string{"node"}, false, Runtime{Kind: RuntimeNode, Exe: "node"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &RuntimeResolver{Dir: t.TempDir(), LookPath: onPath(tc.path...), Mirror: "http://127.0.0.1:0"}
			if tc.localBun {
				writeLocalBun(t, r)
			}
			want := tc.want
			if want.Kind == RuntimeBun && want.Exe == "" {
				want = localBunRuntime(r)
			}
			got, err := r.Resolve(context.Background())
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestResolvePackageManager_NodeWithoutNpmInstallsBun(t *testing.T) {
	srv := newBunServer(t, fakeBunScript, false)
	r := &RuntimeResolver{Dir: t.TempDir(), Mirror: srv.URL, LookPath: onPath("node")}

	rt, err := r.ResolvePackageManager(context.Background())
	require.NoError(t, err)
	assert.Equal(t, localBunRuntime(r), rt)
}

func TestResolve_InstallsLocalBun(t *testing.T) {
	srv := newBunServer(t, fakeBunScript, false)
	var installs []string
	r := &RuntimeResolver{
		Dir:       t.TempDir(),
		Mirror:    srv.URL + "/",
		LookPath:  onPath(),
		OnInstall: func(version, dir string) { installs = append(installs, version+" "+dir) },
	}
	old := filepath.Join(r.Dir, "bun-0.0.1", "bin", "bun")
	require.NoError(t, os.MkdirAll(filepath.Dir(old), 0o755))
	require.NoError(t, os.WriteFile(old, nil, 0o755))
	stale := filepath.Join(r.Dir, "bun-"+BunVersion, "bin", ".bun-123.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte("partial"), 0o644))
	staleTime := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(stale, staleTime, staleTime))
	fresh := filepath.Join(filepath.Dir(stale), ".bun-456.zip")
	require.NoError(t, os.WriteFile(fresh, []byte("in progress"), 0o644))

	rt, err := r.Resolve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, localBunRuntime(r), rt)
	assert.Equal(t, []string{BunVersion + " " + filepath.Join(r.Dir, "bun-"+BunVersion)}, installs)
	out, err := exec.Command(rt.Exe, "--version").Output()
	require.NoError(t, err)
	assert.Equal(t, BunVersion+"\n", string(out))
	var names []string
	entries, err := os.ReadDir(filepath.Dir(rt.Exe))
	require.NoError(t, err)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{".bun-456.zip", "bun"}, names, "only stale temporary files are removed")
	assert.NoDirExists(t, filepath.Join(r.Dir, "bun-0.0.1"))

	_, err = r.Resolve(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, srv.downloads.Load())
	assert.Len(t, installs, 1)
}

func TestResolve_ConcurrentCallsDownloadOnce(t *testing.T) {
	srv := newBunServer(t, fakeBunScript, false)
	r := &RuntimeResolver{Dir: t.TempDir(), Mirror: srv.URL, LookPath: onPath()}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Resolve(context.Background())
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, srv.downloads.Load())
}

func TestResolve_InstallFailures(t *testing.T) {
	cases := []struct {
		name      string
		script    string
		corrupt   bool
		transient bool
		wantErr   string
	}{
		{"checksum mismatch", fakeBunScript, true, true, "checksum mismatch"},
		{"binary does not run", "#!/bin/sh\nexit 1\n", false, false, "does not run"},
		{"wrong version", "#!/bin/sh\necho 0.0.1\n", false, false, "0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newBunServer(t, tc.script, tc.corrupt)
			r := &RuntimeResolver{Dir: t.TempDir(), Mirror: srv.URL, LookPath: onPath()}
			_, err := r.Resolve(context.Background())
			require.Error(t, err)
			assert.Equal(t, tc.transient, errors.Is(err, ErrBunDownload), "got %v", err)
			assert.Contains(t, err.Error(), tc.wantErr)
			entries, err := os.ReadDir(filepath.Join(r.Dir, "bun-"+BunVersion, "bin"))
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}

	t.Run("mirror without the release", func(t *testing.T) {
		hostBunTarget(t)
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)
		r := &RuntimeResolver{Dir: t.TempDir(), Mirror: srv.URL, LookPath: onPath()}
		_, err := r.Resolve(context.Background())
		assert.True(t, errors.Is(err, ErrBunDownload), "got %v", err)
		assert.Contains(t, err.Error(), "404")
	})
}

func TestInstallBun_FallsBackToNextMirror(t *testing.T) {
	srv := newBunServer(t, fakeBunScript, false)
	target := hostBunTarget(t)
	down := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(down.Close)
	orig := bunStallTimeout
	bunStallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { bunStallTimeout = orig })
	release := make(chan struct{})
	stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Write([]byte("PK"))
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(stalled.Close)
	t.Cleanup(func() { close(release) })
	dest := filepath.Join(t.TempDir(), "bin", "bun")

	require.NoError(t, installBun(context.Background(), http.DefaultClient, []string{down.URL, stalled.URL, srv.URL}, target, dest))
	assert.FileExists(t, dest)
	assert.EqualValues(t, 1, srv.downloads.Load())
}

func TestInstallBun_StopsWhenBinaryDoesNotRun(t *testing.T) {
	broken := newBunServer(t, "#!/bin/sh\nexit 1\n", false)
	target := hostBunTarget(t)
	dest := filepath.Join(t.TempDir(), "bin", "bun")

	err := installBun(context.Background(), http.DefaultClient, []string{broken.URL, broken.URL}, target, dest)
	assert.True(t, errors.Is(err, errBunUnusable), "got %v", err)
	assert.EqualValues(t, 1, broken.downloads.Load())
}

func TestInvocation(t *testing.T) {
	var looked []string
	r := &RuntimeResolver{Dir: t.TempDir(), LookPath: func(name string) (string, error) {
		looked = append(looked, name)
		return onPath("bun", "sh")(name)
	}}
	dir := t.TempDir()

	inv, err := r.Invocation(context.Background(), dir, []string{"node", "dist/index.js", "--flag"})
	require.NoError(t, err)
	assert.Equal(t, Invocation{Path: "bun", Args: []string{"dist/index.js", "--flag"}}, inv)

	looked = nil
	inv, err = r.Invocation(context.Background(), dir, []string{"./bin/run", "x"})
	require.NoError(t, err)
	assert.Equal(t, Invocation{Path: filepath.Join(dir, "bin", "run"), Args: []string{"x"}}, inv)
	assert.Empty(t, looked, "non-node commands must not resolve a runtime")

	_, err = r.Invocation(context.Background(), dir, nil)
	assert.Error(t, err)

	var none *RuntimeResolver
	inv, err = none.Invocation(context.Background(), dir, []string{"./run"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "run"), inv.Path)
}
