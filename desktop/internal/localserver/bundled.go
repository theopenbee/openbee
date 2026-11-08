//go:build embedserver

package localserver

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/codemode"
	"github.com/theopenbee/openbee/internal/infra/buildinfo"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

const Bundled = true

const (
	logFile      = "server.log"
	logMaxBytes  = 10 << 20
	probeTimeout = time.Second
	defaultPort  = 19876
	unsetVersion = "0.0.0"
)

type Server struct {
	o   Options
	cfg config.Config
	err error

	mu      sync.Mutex
	booted  bool
	cancel  context.CancelFunc
	stopped chan struct{}
}

func RunChild() {
	codemode.MaybeRunChild()
}

func New(o Options) *Server {
	s := &Server{o: o}
	s.cfg, s.err = loadConfig(o.Language)
	return s
}

func loadConfig(lang string) (config.Config, error) {
	path := config.DefaultConfigPath()
	if path == "" {
		return config.Config{}, errors.New("locate the openbee home directory")
	}
	if lang == "" {
		lang = config.DetectLanguage(os.Getenv)
	}
	if _, err := config.EnsureFile(path, lang); err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("load %s: %w", path, err)
	}
	return cfg, nil
}

func (s *Server) URL() string {
	host := s.cfg.Server.Host
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}
	port := cmp.Or(s.cfg.Server.Port, defaultPort)
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

func (s *Server) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running() || serving(ctx, s.URL()) {
		return nil
	}
	if !s.booted {
		if err := s.boot(); err != nil {
			return err
		}
		s.booted = true
	}
	a, err := app.Boot(s.cfg)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		if err := a.Serve(runCtx); err != nil {
			log.Printf("localserver: %v", err)
		}
		close(stopped)
	}()
	s.cancel, s.stopped = cancel, stopped
	return nil
}

func (s *Server) Stop(timeout time.Duration) {
	s.mu.Lock()
	cancel, stopped := s.cancel, s.stopped
	s.cancel, s.stopped = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(timeout):
		log.Printf("localserver: still stopping after %s", timeout)
	}
}

func (s *Server) running() bool {
	if s.stopped == nil {
		return false
	}
	select {
	case <-s.stopped:
		s.cancel, s.stopped = nil, nil
		return false
	default:
		return true
	}
}

func (s *Server) boot() error {
	importShellPath()
	if s.o.Version != "" && s.o.Version != unsetVersion {
		buildinfo.Set(s.o.Version, "", "")
	}
	var out io.Writer = os.Stderr
	if s.o.LogDir != "" {
		f, err := openLog(filepath.Join(s.o.LogDir, logFile))
		if err != nil {
			return err
		}
		out = f
	}
	return logger.Init(logger.Config{
		Level:  cmp.Or(os.Getenv("OPENBEE_LOG_LEVEL"), "info"),
		Format: "json",
		Output: out,
	})
}

func openLog(path string) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() > logMaxBytes {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, fmt.Errorf("rotate server log: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open server log: %w", err)
	}
	return f, nil
}

func serving(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/config", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body map[string]any
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil
}
