package pluginengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	maxLineBytes   = 16 << 20
	maxLoggedBytes = 4 << 10
	waitDelay      = 3 * time.Second
)

type pluginProcess struct {
	cmd        *exec.Cmd
	proc       *ai.CmdProcess
	log        lineLogger
	stdout     *lineWriter
	stderr     *lineWriter
	resp       *response
	lastStderr string
}

func (a *Adapter) start(ctx context.Context, method string, params any, env func(runtimeEnv []string) []string) (*pluginProcess, error) {
	body, err := encodeRequest(a.spec, method, params)
	if err != nil {
		return nil, err
	}
	inv, err := a.spec.Runtime.Invocation(ctx, a.spec.Dir, a.spec.Command)
	if err != nil {
		return nil, fmt.Errorf("resolve engine plugin command: %w", err)
	}
	cmd := exec.CommandContext(ctx, inv.Path, inv.Args...)
	cmd.Dir = a.spec.Dir
	cmd.Env = env(inv.Env)
	cmd.Stdin = bytes.NewReader(body)
	p := &pluginProcess{cmd: cmd, proc: ai.NewCmdProcess(cmd), log: a.log}
	p.stdout = &lineWriter{max: maxLineBytes, onLine: p.stdoutLine, onOverflow: p.overflow("stdout")}
	p.stderr = &lineWriter{max: maxLineBytes, onLine: p.stderrLine, onOverflow: p.overflow("stderr")}
	cmd.Stdout = p.stdout
	cmd.Stderr = p.stderr
	ai.ConfigureCmd(cmd)
	if runtime.GOOS != "windows" {
		cmd.Cancel = p.proc.Stop
	}
	cmd.WaitDelay = waitDelay
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start engine plugin: %w", err)
	}
	return p, nil
}

func (p *pluginProcess) wait() error {
	err := p.cmd.Wait()
	p.stdout.flush()
	p.stderr.flush()
	if errors.Is(err, exec.ErrWaitDelay) {
		p.log.Warn("engine plugin exited but a child process kept its output open")
		return nil
	}
	return err
}

func (p *pluginProcess) stdoutLine(line []byte) {
	if len(bytes.TrimSpace(line)) == 0 {
		return
	}
	if resp, ok := parseResponse(line); ok {
		p.resp = resp
		return
	}
	p.log.Warn("engine plugin wrote a non-protocol line to stdout", zap.String("line", clip(line)))
}

func (p *pluginProcess) stderrLine(line []byte) {
	text := strings.TrimSpace(clip(line))
	if text == "" {
		return
	}
	p.log.Info(text)
	p.lastStderr = text
}

func (p *pluginProcess) overflow(stream string) func() {
	return func() {
		p.log.Warn("engine plugin output line exceeds the size limit and was dropped",
			zap.String("stream", stream), zap.Int("limit_bytes", maxLineBytes))
	}
}

func noResponseError(waitErr error, lastStderr string) error {
	var exitErr *exec.ExitError
	if waitErr != nil && (!errors.As(waitErr, &exitErr) || exitErr.ExitCode() < 0) {
		return waitErr
	}
	detail := lastStderr
	if detail == "" && waitErr != nil {
		detail = waitErr.Error()
	}
	if detail == "" {
		return errors.New("engine plugin exited without a response")
	}
	return fmt.Errorf("engine plugin exited without a response: %s", detail)
}

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == key || runtime.GOOS == "windows" && strings.EqualFold(k, key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func clip(b []byte) string {
	if len(b) > maxLoggedBytes {
		b = b[:maxLoggedBytes]
	}
	return strings.ToValidUTF8(string(b), "")
}

type lineWriter struct {
	max        int
	onLine     func([]byte)
	onOverflow func()
	buf        []byte
	skipping   bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		chunk, rest, found := bytes.Cut(p, []byte{'\n'})
		w.add(chunk)
		if !found {
			break
		}
		w.end()
		p = rest
	}
	return n, nil
}

func (w *lineWriter) add(b []byte) {
	if w.skipping {
		return
	}
	if len(w.buf)+len(b) > w.max {
		w.buf = nil
		w.skipping = true
		w.onOverflow()
		return
	}
	w.buf = append(w.buf, b...)
}

func (w *lineWriter) end() {
	if !w.skipping {
		if line := bytes.TrimRight(w.buf, "\r"); len(line) > 0 {
			w.onLine(line)
		}
	}
	w.buf = w.buf[:0]
	w.skipping = false
}

func (w *lineWriter) flush() {
	w.end()
	w.buf = nil
}
