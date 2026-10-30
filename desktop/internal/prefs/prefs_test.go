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
)

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

func TestFileTokenStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := NewFileTokenStore(dir)
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
	entries := tokenEntries(t, dir)
	if entries[a] != tokenRecord(ta) || entries[b] != tokenRecord(tb) || len(entries) != 2 {
		t.Fatalf("tokens.json = %v", entries)
	}

	fresh := NewFileTokenStore(dir)
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

func TestFileTokenStoreRawFormat(t *testing.T) {
	dir := t.TempDir()
	server := "https://bee.example.com"
	if err := NewFileTokenStore(dir).Save(server, Tokens{Access: "access-token", Refresh: "refresh-token"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tokens.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var onDisk map[string]map[string]string
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("tokens.json is not a JSON map: %v", err)
	}
	record := onDisk[server]
	if record["access"] != "access-token" || record["refresh"] != "refresh-token" || len(record) != 2 {
		t.Fatalf("tokens.json = %s", raw)
	}
}

func TestFileTokenStoreClearWithoutFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := NewFileTokenStore(dir)
	if err := s.Clear("https://a.example.com"); err != nil {
		t.Fatalf("Clear with no tokens file: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Clear created the tokens dir: %v", err)
	}
}

func TestFileTokenStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tokens.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewFileTokenStore(dir)
	server := "https://bee.example.com"

	if _, err := s.Load(server); !errors.Is(err, errCorruptTokens) {
		t.Fatalf("Load of a corrupt tokens.json error = %v, want %v", err, errCorruptTokens)
	}

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
	if entries := tokenEntries(t, dir); len(entries) != 0 {
		t.Fatalf("tokens.json after Clear = %v, want empty", entries)
	}
}

func TestFileTokenStoreReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tokens.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	s := NewFileTokenStore(dir)
	server := "https://bee.example.com"
	if _, err := s.Load(server); err == nil || errors.Is(err, errCorruptTokens) {
		t.Fatalf("Load error = %v, want a read error", err)
	}
	if err := s.Save(server, Tokens{Access: "a", Refresh: "r"}); err == nil {
		t.Fatal("Save over an unreadable tokens.json returned no error")
	}
	if err := s.Clear(server); err == nil {
		t.Fatal("Clear over an unreadable tokens.json returned no error")
	}
}

func TestFileTokenStoreConcurrent(t *testing.T) {
	s := NewFileTokenStore(t.TempDir())
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

func tokenEntries(t *testing.T, dir string) map[string]tokenRecord {
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
