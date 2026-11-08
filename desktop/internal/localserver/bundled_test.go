//go:build embedserver

package localserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "localserver-home")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("SHELL", "/bin/sh")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func withPort(t *testing.T, port int) {
	t.Helper()
	path := config.DefaultConfigPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := config.EnsureFile(path, "en"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "port: 8080", fmt.Sprintf("port: %d", port), 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newServer(t *testing.T) *Server {
	t.Helper()
	s := New(Options{Version: "v0.0.0-test", Language: "en", LogDir: t.TempDir()})
	if s.err != nil {
		t.Fatal(s.err)
	}
	return s
}

func TestStartServesUntilStop(t *testing.T) {
	port := freePort(t)
	withPort(t, port)
	s := newServer(t)
	if want := fmt.Sprintf("http://localhost:%d", port); s.URL() != want {
		t.Fatalf("URL() = %q, want %q", s.URL(), want)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !serving(context.Background(), s.URL()) {
		t.Fatal("the local server does not answer after Start")
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	s.Stop(20 * time.Second)
	if serving(context.Background(), s.URL()) {
		t.Fatal("the local server still answers after Stop")
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start after Stop: %v", err)
	}
	if !serving(context.Background(), s.URL()) {
		t.Fatal("the local server does not answer after a restart")
	}
	s.Stop(20 * time.Second)
}

func TestStartIgnoresOtherServersOnThePort(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>dev server</title>"))
	}))
	t.Cleanup(other.Close)
	withPort(t, other.Listener.Addr().(*net.TCPAddr).Port)
	s := newServer(t)
	if serving(context.Background(), s.URL()) {
		t.Fatal("serving() took an HTML dev server for OpenBee")
	}
	if err := s.Start(context.Background()); err == nil {
		s.Stop(20 * time.Second)
		t.Fatal("Start = nil while another program holds the port")
	}
}

func TestStartReportsBrokenConfig(t *testing.T) {
	path := config.DefaultConfigPath()
	if err := os.WriteFile(path, []byte("server: [not, a, map"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Language: "en", LogDir: t.TempDir()})
	if s.URL() != "http://localhost:8080" {
		t.Fatalf("URL() with a broken config = %q, want the default http://localhost:8080", s.URL())
	}
	err := s.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Start = %v, want an error naming %s", err, path)
	}
}

func TestOpenLogRotatesLargeLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), logFile)
	if err := os.WriteFile(path, make([]byte, logMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("log after rotation: %v, %v; want a new empty file", info, err)
	}
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != logMaxBytes+1 {
		t.Fatalf("rotated log: %v, %v; want the old content", info, err)
	}
}

func TestStartAttachesToRunningServer(t *testing.T) {
	running := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(running.Close)
	withPort(t, running.Listener.Addr().(*net.TCPAddr).Port)
	s := newServer(t)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.stopped != nil || s.booted {
		t.Fatal("Start booted its own server although one was already running")
	}
	s.Stop(time.Second)
	if !serving(context.Background(), s.URL()) {
		t.Fatal("Stop shut down a server the desktop did not start")
	}
}

func TestStartReportsBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	withPort(t, ln.Addr().(*net.TCPAddr).Port)
	s := newServer(t)
	if err := s.Start(context.Background()); err == nil {
		s.Stop(20 * time.Second)
		t.Fatal("Start = nil on a port another program holds")
	}
	if s.stopped != nil {
		t.Fatal("a failed Start left the server marked as running")
	}
}

func TestLoginShellPathSkipsShellOutput(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "fakeshell")
	script := "#!/bin/sh\necho 'welcome banner'\nPATH=/opt/tools/bin:/usr/bin\nexport PATH\n/bin/sh -c \"$2\"\necho 'goodbye'\n"
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := loginShellPath(shell, 5*time.Second)
	if err != nil {
		t.Fatalf("loginShellPath: %v", err)
	}
	if got != "/opt/tools/bin:/usr/bin" {
		t.Fatalf("loginShellPath = %q, want /opt/tools/bin:/usr/bin", got)
	}
}

func TestLoginShellPathTimesOut(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "slowshell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := loginShellPath(shell, 200*time.Millisecond); err == nil {
		t.Fatal("loginShellPath = nil for a shell that never answers")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("loginShellPath took %v, want it cut off by the timeout", d)
	}
}

func TestMergePathsKeepsShellOrderFirst(t *testing.T) {
	got := mergePaths("/opt/homebrew/bin:/usr/bin", "/usr/bin:/bin::/usr/sbin")
	want := "/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin"
	if got != want {
		t.Fatalf("mergePaths = %q, want %q", got, want)
	}
}

func TestURLReachesWildcardHosts(t *testing.T) {
	cases := map[string]string{
		"":          "http://localhost:8080",
		"0.0.0.0":   "http://localhost:8080",
		"::":        "http://localhost:8080",
		"localhost": "http://localhost:8080",
		"127.0.0.1": "http://127.0.0.1:8080",
		"::1":       "http://[::1]:8080",
	}
	for host, want := range cases {
		s := &Server{cfg: config.Config{Server: config.ServerConfig{Host: host, Port: 8080}}}
		if got := s.URL(); got != want {
			t.Errorf("URL() with host %q = %q, want %q", host, got, want)
		}
	}
}
