package pluginchannel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/plugin"
)

var ErrNotRunning = errors.New("plugin channel is not running")

type Spec struct {
	ChannelID string
	Dir       string
	Command   []string
	Env       []string
	Runtime   *plugin.RuntimeResolver
	Init      InitializeParams
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

type Supervisor struct {
	spec      Spec
	onNotify  NotificationHandler
	log       *logger.Logger
	pluginLog *logger.Logger

	initTimeout     time.Duration
	initialBackoff  time.Duration
	maxBackoff      time.Duration
	stableAfter     time.Duration
	shutdownTimeout time.Duration
	killGrace       time.Duration

	mu   sync.Mutex
	conn *Conn
	done chan struct{}
}

func NewSupervisor(spec Spec, onNotify NotificationHandler) *Supervisor {
	return &Supervisor{
		spec:            spec,
		onNotify:        onNotify,
		log:             log.With(zap.String("channel", spec.ChannelID)),
		pluginLog:       logger.With(zap.String("component", "plugin:"+spec.ChannelID)),
		initTimeout:     30 * time.Second,
		initialBackoff:  time.Second,
		maxBackoff:      60 * time.Second,
		stableAfter:     10 * time.Minute,
		shutdownTimeout: 5 * time.Second,
		killGrace:       2 * time.Second,
		done:            make(chan struct{}),
	}
}

func (s *Supervisor) Done() <-chan struct{} { return s.done }

func (s *Supervisor) Call(ctx context.Context, method string, params, result any) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("channel %s: %w", s.spec.ChannelID, ErrNotRunning)
	}
	err := conn.Call(ctx, method, params, result)
	if errors.Is(err, ErrConnClosed) {
		return fmt.Errorf("channel %s: %w", s.spec.ChannelID, ErrNotRunning)
	}
	return err
}

func (s *Supervisor) setConn(c *Conn) {
	s.mu.Lock()
	s.conn = c
	s.mu.Unlock()
}

func (s *Supervisor) Run(ctx context.Context) {
	defer close(s.done)
	backoff := s.initialBackoff
	for {
		started := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		var perm *permanentError
		if errors.As(err, &perm) {
			s.log.Error("plugin channel disabled", zap.Error(err))
			return
		}
		if time.Since(started) >= s.stableAfter {
			backoff = s.initialBackoff
		}
		s.log.Warn("plugin exited, restarting", zap.Error(err), zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

type child struct {
	proc     *ai.CmdProcess
	stdin    io.WriteCloser
	conn     *Conn
	waitDone chan error
}

func (s *Supervisor) start(ctx context.Context) (*child, error) {
	if len(s.spec.Command) == 0 {
		return nil, &permanentError{errors.New("plugin command is empty")}
	}
	inv, err := s.spec.Runtime.Invocation(ctx, s.spec.Dir, s.spec.Command)
	if errors.Is(err, plugin.ErrBunDownload) {
		return nil, fmt.Errorf("resolve plugin command: %w", err)
	}
	if err != nil {
		return nil, &permanentError{fmt.Errorf("resolve plugin command: %w", err)}
	}
	cmd := exec.Command(inv.Path, inv.Args...)
	cmd.Dir = s.spec.Dir
	cmd.Env = append(append(os.Environ(), inv.Env...), s.spec.Env...)
	ai.ConfigureCmd(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) || errors.Is(err, exec.ErrNotFound) {
			return nil, &permanentError{fmt.Errorf("start plugin: %w", err)}
		}
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	c := &child{proc: ai.NewCmdProcess(cmd), stdin: stdin, waitDone: make(chan error, 1)}
	c.conn = NewConn(stdout, stdin, s.handleNotify, s.log)
	connDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		defer close(connDone)
		if err := c.conn.Run(); err != nil {
			s.log.Warn("plugin stdout read failed", zap.Error(err))
		}
	}()
	go func() {
		defer close(stderrDone)
		s.pipeStderr(stderr)
	}()
	go func() {
		<-connDone
		<-stderrDone
		c.waitDone <- cmd.Wait()
	}()
	return c, nil
}

func (s *Supervisor) runOnce(ctx context.Context) error {
	c, err := s.start(ctx)
	if err != nil {
		return err
	}

	initCtx, cancel := context.WithTimeout(ctx, s.initTimeout)
	var res InitializeResult
	err = c.conn.Call(initCtx, "initialize", s.spec.Init, &res)
	cancel()
	if err != nil {
		s.kill(c)
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == CodeConfigInvalid {
			return &permanentError{fmt.Errorf("plugin rejected its config: %s", rpcErr.Message)}
		}
		return fmt.Errorf("initialize plugin: %w", err)
	}
	if res.ProtocolVersion != ProtocolVersion {
		s.kill(c)
		return &permanentError{fmt.Errorf("plugin speaks protocol_version %d, openbee supports %d", res.ProtocolVersion, ProtocolVersion)}
	}

	s.setConn(c.conn)
	s.log.Info("plugin channel started")
	select {
	case err := <-c.waitDone:
		s.setConn(nil)
		return fmt.Errorf("plugin exited: %v", err)
	case <-ctx.Done():
		s.setConn(nil)
		s.stop(c)
		return ctx.Err()
	}
}

func (s *Supervisor) stop(c *child) {
	sctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	err := c.conn.Call(sctx, "shutdown", nil, nil)
	cancel()
	if err != nil && !errors.Is(err, ErrConnClosed) {
		s.log.Warn("plugin shutdown request failed", zap.Error(err))
	}
	_ = c.stdin.Close()
	select {
	case <-c.waitDone:
		return
	case <-time.After(s.killGrace):
	}
	s.kill(c)
}

func (s *Supervisor) kill(c *child) {
	_ = c.stdin.Close()
	if err := c.proc.Stop(); err != nil {
		s.log.Warn("kill plugin failed", zap.Error(err))
	}
	select {
	case <-c.waitDone:
	case <-time.After(5 * time.Second):
		s.log.Warn("plugin did not exit after kill")
	}
}

func (s *Supervisor) pipeStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLineBytes)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), "\r"); line != "" {
			s.pluginLog.Info(line)
		}
	}
	_, _ = io.Copy(io.Discard, r)
}

func (s *Supervisor) handleNotify(method string, params json.RawMessage) {
	if method == "log" {
		var p LogParams
		if json.Unmarshal(params, &p) != nil {
			return
		}
		fields := make([]zap.Field, 0, len(p.Fields))
		for k, v := range p.Fields {
			fields = append(fields, zap.Any(k, v))
		}
		switch p.Level {
		case "debug":
			s.pluginLog.Debug(p.Message, fields...)
		case "warn":
			s.pluginLog.Warn(p.Message, fields...)
		case "error":
			s.pluginLog.Error(p.Message, fields...)
		default:
			s.pluginLog.Info(p.Message, fields...)
		}
		return
	}
	if s.onNotify != nil {
		s.onNotify(method, params)
	}
}
