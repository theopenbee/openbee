package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/egoist/mygo"
)

type fakeSecrets struct {
	mu   sync.Mutex
	data map[string][]byte
	err  error
}

func newFakeSecrets() *fakeSecrets {
	return &fakeSecrets{data: map[string][]byte{}}
}

func (f *fakeSecrets) Set(key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.data[key] = append([]byte(nil), value...)
	return nil
}

func (f *fakeSecrets) Get(key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.data[key]
	if !ok {
		return nil, mygo.ErrSecretNotFound
	}
	return append([]byte(nil), v...), nil
}

func (f *fakeSecrets) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if _, ok := f.data[key]; !ok {
		return mygo.ErrSecretNotFound
	}
	delete(f.data, key)
	return nil
}

func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode of %s = %v, want %v", path, got, want)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "prefs.json")
	s := NewFileStore(path)

	if got := s.Load(); got != (Prefs{}) {
		t.Fatalf("Load on missing file = %+v, want zero Prefs", got)
	}

	want := Prefs{
		ServerURL:                  "https://bee.example.com",
		Username:                   "ada",
		Theme:                      "dark",
		Language:                   "zh-CN",
		Notifications:              "all",
		NotificationsHintDismissed: true,
		NotificationsDenied:        true,
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Load(); got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
	if got := NewFileStore(path).Load(); got != want {
		t.Fatalf("Load from fresh store = %+v, want %+v", got, want)
	}
	requireMode(t, path, 0o600)
	requireMode(t, filepath.Dir(path), 0o700)

	want.Theme = "light"
	if err := s.Save(want); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got := s.Load(); got != want {
		t.Fatalf("Load after overwrite = %+v, want %+v", got, want)
	}
	requireMode(t, path, 0o600)

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want only the prefs file", len(entries))
	}
}

func TestFileStoreCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	for _, content := range []string{"not json at all", `{"server_url": "https://x", "theme": `, `[1,2,3]`, ""} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if got := NewFileStore(path).Load(); got != (Prefs{}) {
			t.Fatalf("Load of %q = %+v, want zero Prefs", content, got)
		}
	}
}

func TestFileStoreConcurrent(t *testing.T) {
	s := NewFileStore(filepath.Join(t.TempDir(), "prefs.json"))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := Prefs{Username: fmt.Sprintf("user-%d", i)}
			if err := s.Save(p); err != nil {
				t.Errorf("Save: %v", err)
			}
			s.Load()
		}(i)
	}
	wg.Wait()
	if got := s.Load(); got.Username == "" {
		t.Fatalf("Load after concurrent saves = %+v, want a saved value", got)
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore(Prefs{Theme: "dark"})
	if got := s.Load(); got.Theme != "dark" {
		t.Fatalf("Load = %+v, want theme dark", got)
	}
	if err := s.Save(Prefs{Theme: "light", Username: "ada"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Load(); got != (Prefs{Theme: "light", Username: "ada"}) {
		t.Fatalf("Load = %+v", got)
	}
}

func TestMemoryTokenStore(t *testing.T) {
	s := NewMemoryTokenStore()
	a, b := "https://a.example.com", "https://b.example.com"

	got, err := s.Load(a)
	if err != nil || got != (Tokens{}) {
		t.Fatalf("Load of unsaved server = %+v, %v; want zero Tokens, nil", got, err)
	}

	if err := s.Save(a, Tokens{Access: "a-access", Refresh: "a-refresh"}); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	if err := s.Save(b, Tokens{Access: "b-access", Refresh: "b-refresh"}); err != nil {
		t.Fatalf("Save b: %v", err)
	}
	if got, _ := s.Load(a); got != (Tokens{Access: "a-access", Refresh: "a-refresh"}) {
		t.Fatalf("Load a = %+v", got)
	}
	if got, _ := s.Load(b); got != (Tokens{Access: "b-access", Refresh: "b-refresh"}) {
		t.Fatalf("Load b = %+v", got)
	}

	if err := s.Clear(a); err != nil {
		t.Fatalf("Clear a: %v", err)
	}
	if got, _ := s.Load(a); got != (Tokens{}) {
		t.Fatalf("Load a after Clear = %+v, want zero", got)
	}
	if got, _ := s.Load(b); got.Access != "b-access" {
		t.Fatalf("Clear a affected b: %+v", got)
	}
	if err := s.Clear(a); err != nil {
		t.Fatalf("Clear of missing entry: %v", err)
	}
}

func TestMemoryTokenStoreConcurrent(t *testing.T) {
	s := NewMemoryTokenStore()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			server := fmt.Sprintf("https://s%d.example.com", i%4)
			_ = s.Save(server, Tokens{Access: "x", Refresh: "y"})
			_, _ = s.Load(server)
			_ = s.Clear(server)
		}(i)
	}
	wg.Wait()
}

func TestSecretStoreUsesBackend(t *testing.T) {
	dir := t.TempDir()
	backend := newFakeSecrets()
	s := newSecretTokenStore(backend, dir)
	server := "https://bee.example.com"

	got, err := s.Load(server)
	if err != nil || got != (Tokens{}) {
		t.Fatalf("Load of unsaved server = %+v, %v; want zero Tokens, nil", got, err)
	}

	want := Tokens{Access: "access-token", Refresh: "refresh-token"}
	if err := s.Save(server, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, ok := backend.data["tokens:"+server]
	if !ok {
		t.Fatalf("backend keys = %v, want tokens:%s", backend.data, server)
	}
	var record map[string]string
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("backend value is not JSON: %v", err)
	}
	if record["access"] != "access-token" || record["refresh"] != "refresh-token" || len(record) != 2 {
		t.Fatalf("backend value = %v", record)
	}
	if len(raw) >= mygo.MaxSecretSize {
		t.Fatalf("backend value is %d bytes, want < %d", len(raw), mygo.MaxSecretSize)
	}

	if got, err := s.Load(server); err != nil || got != want {
		t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.json")); !os.IsNotExist(err) {
		t.Fatalf("fallback file exists although the backend works: %v", err)
	}

	if err := s.Clear(server); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok := backend.data["tokens:"+server]; ok {
		t.Fatal("Clear left the secret in the backend")
	}
	if err := s.Clear(server); err != nil {
		t.Fatalf("Clear of missing secret: %v", err)
	}
	if got, err := s.Load(server); err != nil || got != (Tokens{}) {
		t.Fatalf("Load after Clear = %+v, %v", got, err)
	}
}

func TestSecretStoreFallback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, dir)
	a, b := "https://a.example.com", "https://b.example.com"

	if got, err := s.Load(a); err != nil || got != (Tokens{}) {
		t.Fatalf("Load before any save = %+v, %v; want zero Tokens, nil", got, err)
	}

	ta := Tokens{Access: "a-access", Refresh: "a-refresh"}
	tb := Tokens{Access: "b-access", Refresh: "b-refresh"}
	if err := s.Save(a, ta); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	if err := s.Save(b, tb); err != nil {
		t.Fatalf("Save b: %v", err)
	}

	file := filepath.Join(dir, "tokens.json")
	requireMode(t, file, 0o600)
	requireMode(t, dir, 0o700)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var onDisk map[string]map[string]string
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("tokens.json is not a JSON map: %v", err)
	}
	if onDisk[a]["access"] != "a-access" || onDisk[a]["refresh"] != "a-refresh" ||
		onDisk[b]["access"] != "b-access" || onDisk[b]["refresh"] != "b-refresh" {
		t.Fatalf("tokens.json = %s", raw)
	}

	fresh := newSecretTokenStore(backend, dir)
	if got, err := fresh.Load(a); err != nil || got != ta {
		t.Fatalf("Load a from fresh store = %+v, %v; want %+v", got, err, ta)
	}
	if got, err := fresh.Load(b); err != nil || got != tb {
		t.Fatalf("Load b from fresh store = %+v, %v; want %+v", got, err, tb)
	}

	if err := fresh.Clear(a); err != nil {
		t.Fatalf("Clear a: %v", err)
	}
	if got, _ := fresh.Load(a); got != (Tokens{}) {
		t.Fatalf("Load a after Clear = %+v, want zero", got)
	}
	if got, _ := fresh.Load(b); got != tb {
		t.Fatalf("Clear a affected b: %+v", got)
	}
	if err := fresh.Clear(a); err != nil {
		t.Fatalf("Clear of missing entry: %v", err)
	}
	if err := fresh.Clear("https://never.example.com"); err != nil {
		t.Fatalf("Clear of never-saved server: %v", err)
	}
	requireMode(t, file, 0o600)
}

func TestSecretStoreClearWithoutFallbackFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, dir)
	if err := s.Clear("https://a.example.com"); err != nil {
		t.Fatalf("Clear with no fallback file: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Clear created the fallback dir: %v", err)
	}
}

func TestSecretStoreBackendErrorIsNotMasked(t *testing.T) {
	dir := t.TempDir()
	boom := errors.New("keychain locked")
	backend := newFakeSecrets()
	backend.err = boom
	s := newSecretTokenStore(backend, dir)
	server := "https://bee.example.com"

	if err := s.Save(server, Tokens{Access: "a", Refresh: "r"}); !errors.Is(err, boom) {
		t.Fatalf("Save error = %v, want %v", err, boom)
	}
	if _, err := s.Load(server); !errors.Is(err, boom) {
		t.Fatalf("Load error = %v, want %v", err, boom)
	}
	if err := s.Clear(server); !errors.Is(err, boom) {
		t.Fatalf("Clear error = %v, want %v", err, boom)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.json")); !os.IsNotExist(err) {
		t.Fatalf("fallback file written after a non-unsupported error: %v", err)
	}
}

func TestSecretStoreCorruptValue(t *testing.T) {
	backend := newFakeSecrets()
	server := "https://bee.example.com"
	backend.data["tokens:"+server] = []byte("not json")
	s := newSecretTokenStore(backend, t.TempDir())
	if _, err := s.Load(server); err == nil {
		t.Fatal("Load of a corrupt secret returned no error")
	}
}

func TestSecretStoreConcurrent(t *testing.T) {
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			server := fmt.Sprintf("https://s%d.example.com", i)
			want := Tokens{Access: fmt.Sprintf("a%d", i), Refresh: fmt.Sprintf("r%d", i)}
			if err := s.Save(server, want); err != nil {
				t.Errorf("Save: %v", err)
				return
			}
			if got, err := s.Load(server); err != nil || got != want {
				t.Errorf("Load %d = %+v, %v; want %+v", i, got, err, want)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < 16; i++ {
		server := fmt.Sprintf("https://s%d.example.com", i)
		want := Tokens{Access: fmt.Sprintf("a%d", i), Refresh: fmt.Sprintf("r%d", i)}
		if got, err := s.Load(server); err != nil || got != want {
			t.Fatalf("final Load %d = %+v, %v; want %+v", i, got, err, want)
		}
	}
}

func TestNewSecretTokenStoreDoesNotTouchKeychain(t *testing.T) {
	if NewSecretTokenStore(t.TempDir()) == nil {
		t.Fatal("NewSecretTokenStore returned nil")
	}
}

func fallbackEntries(t *testing.T, dir string) map[string]tokenRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "tokens.json"))
	if err != nil {
		t.Fatalf("read tokens.json: %v", err)
	}
	var entries map[string]tokenRecord
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("tokens.json is not a JSON map: %v (%s)", err, raw)
	}
	return entries
}

func TestSecretStoreClearRemovesFallbackOnceKeychainWorks(t *testing.T) {
	dir := t.TempDir()
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, dir)
	server, other := "https://bee.example.com", "https://other.example.com"
	if err := s.Save(server, Tokens{Access: "a", Refresh: "r"}); err != nil {
		t.Fatalf("fallback Save: %v", err)
	}
	if err := s.Save(other, Tokens{Access: "oa", Refresh: "or"}); err != nil {
		t.Fatalf("fallback Save: %v", err)
	}

	backend.err = nil
	if err := s.Clear(server); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	entries := fallbackEntries(t, dir)
	if _, ok := entries[server]; ok {
		t.Fatalf("tokens.json still holds %s after Clear: %v", server, entries)
	}
	if entries[other] != (tokenRecord{Access: "oa", Refresh: "or"}) {
		t.Fatalf("Clear of %s touched %s: %v", server, other, entries)
	}
}

func TestSecretStoreSaveDropsStaleFallback(t *testing.T) {
	dir := t.TempDir()
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, dir)
	server := "https://bee.example.com"
	if err := s.Save(server, Tokens{Access: "old", Refresh: "old-refresh"}); err != nil {
		t.Fatalf("fallback Save: %v", err)
	}

	backend.err = nil
	want := Tokens{Access: "new", Refresh: "new-refresh"}
	if err := s.Save(server, want); err != nil {
		t.Fatalf("keychain Save: %v", err)
	}
	if _, ok := fallbackEntries(t, dir)[server]; ok {
		t.Fatal("tokens.json still holds the server after a keychain save")
	}
	if got, err := s.Load(server); err != nil || got != want {
		t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
	}
}

func TestSecretStoreRewritesCorruptFallback(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tokens.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := newFakeSecrets()
	backend.err = errors.ErrUnsupported
	s := newSecretTokenStore(backend, dir)
	server := "https://bee.example.com"

	want := Tokens{Access: "a", Refresh: "r"}
	if err := s.Save(server, want); err != nil {
		t.Fatalf("Save over a corrupt tokens.json: %v", err)
	}
	if got, err := s.Load(server); err != nil || got != want {
		t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
	}

	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(server); err != nil {
		t.Fatalf("Clear over a corrupt tokens.json: %v", err)
	}
	if entries := fallbackEntries(t, dir); len(entries) != 0 {
		t.Fatalf("tokens.json after Clear = %v, want empty", entries)
	}
}
