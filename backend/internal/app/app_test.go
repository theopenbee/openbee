package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "app-home")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func testConfig(t *testing.T, port int) config.Config {
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
	data = []byte(strings.Replace(string(data), "port: 19876", fmt.Sprintf("port: %d", port), 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
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

func TestServeStopsWhenContextEnds(t *testing.T) {
	cfg := testConfig(t, freePort(t))
	a, err := Boot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx) }()

	resp, err := http.Get(cfg.Bee.RPCBaseURL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/config = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve = %v, want nil", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
	if _, err := http.Get(cfg.Bee.RPCBaseURL + "/api/config"); err == nil {
		t.Fatal("server still answers after shutdown")
	}
}

func TestBootLeavesTheDatabaseAloneWhenThePortIsBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := testConfig(t, ln.Addr().(*net.TCPAddr).Port)
	if err := os.RemoveAll(cfg.Database.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := Boot(cfg); err == nil {
		t.Fatal("Boot = nil on a busy port")
	}
	if _, err := os.Stat(cfg.Database.Path); !os.IsNotExist(err) {
		t.Fatalf("Boot touched the database before it had the port: stat = %v", err)
	}
}
