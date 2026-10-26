package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogJSON(version string) []byte {
	data, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"categories":     []any{map[string]any{"id": "channel", "name": map[string]string{"en": "Channels", "zh": "渠道"}}},
		"tags":           []any{},
		"plugins": []any{map[string]any{
			"category":         "channel",
			"id":               "telegram",
			"package":          "@theopenbee/channel-telegram",
			"version":          version,
			"official":         true,
			"name":             map[string]string{"en": "Telegram"},
			"protocol_version": 1,
			"license":          "MIT",
			"repository":       "https://github.com/theopenbee/channel-telegram",
			"maintainers":      []string{"Qsnh"},
			"tags":             []string{"im"},
			"integrity":        "sha512-x",
			"tarball":          "https://registry.npmjs.org/x.tgz",
		}},
	})
	return data
}

func makeTgz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func integrityOf(b []byte) string {
	sum := sha512.Sum512(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

type fakeRegistry struct {
	srv       *httptest.Server
	versions  map[string][]byte
	integrity map[string]string
	down      atomic.Bool
	hits      atomic.Int32
}

func newFakeRegistry(t *testing.T, versions map[string]string) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{versions: map[string][]byte{}, integrity: map[string]string{}}
	for v, catalogVersion := range versions {
		tgz := makeTgz(t, map[string][]byte{"package/catalog.json": catalogJSON(catalogVersion), "package/package.json": []byte(`{}`)})
		r.versions[v] = tgz
		r.integrity[v] = integrityOf(tgz)
	}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		if r.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch {
		case req.URL.Path == "/@theopenbee%2Fplugins" || req.URL.Path == "/@theopenbee/plugins":
			doc := map[string]any{"name": PackageName, "versions": map[string]any{}}
			for v := range r.versions {
				doc["versions"].(map[string]any)[v] = map[string]any{"dist": map[string]string{
					"tarball":   fmt.Sprintf("%s/tarballs/%s.tgz", r.srv.URL, v),
					"integrity": r.integrity[v],
				}}
			}
			_ = json.NewEncoder(w).Encode(doc)
		case filepath.Dir(req.URL.Path) == "/tarballs":
			v := filepath.Base(req.URL.Path)
			v = v[:len(v)-len(".tgz")]
			_, _ = w.Write(r.versions[v])
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func newTestClient(t *testing.T, registries ...string) *Client {
	return NewClient(Options{Registries: registries, CachePath: filepath.Join(t.TempDir(), "catalog.json")})
}

func TestClient_PicksHighestStableV1(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{"1.0.2": "0.1.2", "1.0.10": "0.1.10", "1.1.0-beta.1": "beta", "2.0.0": "two", "0.9.0": "old"})
	cat, err := newTestClient(t, reg.srv.URL).Get(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 1, cat.SchemaVersion)
	require.Len(t, cat.Plugins, 1)
	assert.Equal(t, "0.1.10", cat.Plugins[0].Version)
	assert.Equal(t, "Telegram", cat.Plugins[0].Name.Get("en"))
	assert.Equal(t, "渠道", cat.Categories[0].Name.Get("zh"))
}

func TestClient_VerifiesIntegrity(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{"1.0.0": "0.1.0"})
	reg.integrity["1.0.0"] = integrityOf([]byte("something else"))
	_, err := newTestClient(t, reg.srv.URL).Get(context.Background(), false)
	assert.ErrorContains(t, err, "integrity")
}

func TestClient_FallsBackToMirror(t *testing.T) {
	primary := newFakeRegistry(t, map[string]string{"1.0.5": "0.1.5"})
	primary.down.Store(true)
	mirror := newFakeRegistry(t, map[string]string{"1.0.4": "0.1.4"})
	cat, err := newTestClient(t, primary.srv.URL, mirror.srv.URL).Get(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "0.1.4", cat.Plugins[0].Version)
}

func TestClient_UsesDiskCacheWhenOffline(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{"1.0.0": "0.1.0"})
	cachePath := filepath.Join(t.TempDir(), "catalog.json")
	_, err := NewClient(Options{Registries: []string{reg.srv.URL}, CachePath: cachePath}).Get(context.Background(), false)
	require.NoError(t, err)
	assert.FileExists(t, cachePath)

	reg.down.Store(true)
	cat, err := NewClient(Options{Registries: []string{reg.srv.URL}, CachePath: cachePath}).Get(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", cat.Plugins[0].Version)
}

func TestClient_NoCacheReturnsUnavailable(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{"1.0.0": "0.1.0"})
	reg.down.Store(true)
	_, err := newTestClient(t, reg.srv.URL).Get(context.Background(), false)
	assert.Error(t, err)
}

func TestClient_NotPublished(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{})
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err := newTestClient(t, srv.URL).Get(context.Background(), false)
	assert.ErrorContains(t, err, "not published")
	_, err = newTestClient(t, reg.srv.URL).Get(context.Background(), false)
	assert.ErrorContains(t, err, "no 1.x release")
}

func TestClient_MemoryCacheTTL(t *testing.T) {
	reg := newFakeRegistry(t, map[string]string{"1.0.0": "0.1.0"})
	now := time.Now()
	c := NewClient(Options{Registries: []string{reg.srv.URL}, CachePath: filepath.Join(t.TempDir(), "c.json"), Now: func() time.Time { return now }})
	_, err := c.Get(context.Background(), false)
	require.NoError(t, err)
	hits := reg.hits.Load()

	_, err = c.Get(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, hits, reg.hits.Load())

	_, err = c.Get(context.Background(), true)
	require.NoError(t, err)
	assert.Greater(t, reg.hits.Load(), hits)
	hits = reg.hits.Load()

	now = now.Add(11 * time.Minute)
	_, err = c.Get(context.Background(), false)
	require.NoError(t, err)
	assert.Greater(t, reg.hits.Load(), hits)
}

func TestClient_OverrideFileAndURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, catalogJSON("0.3.0"), 0o644))
	cat, err := NewClient(Options{Override: path, CachePath: filepath.Join(t.TempDir(), "c.json")}).Get(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "0.3.0", cat.Plugins[0].Version)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(catalogJSON("0.4.0")) }))
	t.Cleanup(srv.Close)
	cat, err = NewClient(Options{Override: srv.URL + "/catalog.json", CachePath: filepath.Join(t.TempDir(), "c.json")}).Get(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "0.4.0", cat.Plugins[0].Version)
}

func TestClient_RejectsOtherSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schema_version":2,"plugins":[]}`), 0o644))
	_, err := NewClient(Options{Override: path, CachePath: filepath.Join(t.TempDir(), "c.json")}).Get(context.Background(), false)
	assert.ErrorContains(t, err, "schema_version")
}
