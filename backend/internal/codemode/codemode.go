package codemode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	Timeout         = 50 * time.Second
	childEnv        = "OPENBEE_CODEMODE_CHILD"
	protocolVersion = 1
	killGrace       = 3 * time.Second
	stderrLimit     = 4096
	maxMessageBytes = 4 << 20
	crashedError    = "the script crashed"
)

var errMessageTooLarge = fmt.Errorf("the script sent more than %d MiB at once", maxMessageBytes>>20)

type CallFunc func(ctx context.Context, name string, args json.RawMessage) (any, error)

type request struct {
	Version     int           `json:"version"`
	Code        string        `json:"code"`
	Methods     []string      `json:"methods"`
	Timeout     time.Duration `json:"timeout"`
	MemoryLimit uint64        `json:"memory_limit"`
}

type reply struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type childCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type childMessage struct {
	Call   *childCall `json:"call,omitempty"`
	Done   bool       `json:"done,omitempty"`
	Output string     `json:"output,omitempty"`
	Error  string     `json:"error,omitempty"`
}

func Run(ctx context.Context, code string, methods []string, call CallFunc) (string, error) {
	return run(ctx, code, methods, call, Timeout)
}

func run(ctx context.Context, code string, methods []string, call CallFunc, timeout time.Duration) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate openbee binary: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+killGrace)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = []string{childEnv + "=1"}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start script runner: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	lines := bufio.NewReader(stdout)
	req := request{Version: protocolVersion, Code: code, Methods: methods, Timeout: timeout, MemoryLimit: memoryLimit}
	if err := enc.Encode(req); err != nil {
		return "", childFailure(ctx, cmd, stderr, timeout)
	}
	for {
		line, err := readLine(lines, maxMessageBytes)
		if errors.Is(err, errMessageTooLarge) {
			return "", err
		}
		var msg childMessage
		if err != nil || json.Unmarshal(line, &msg) != nil {
			return "", childFailure(ctx, cmd, stderr, timeout)
		}
		if msg.Call == nil {
			if msg.Error != "" {
				return msg.Output, errors.New(msg.Error)
			}
			return msg.Output, nil
		}
		if err := enc.Encode(serve(ctx, methods, call, msg.Call)); err != nil {
			return "", childFailure(ctx, cmd, stderr, timeout)
		}
	}
}

func readLine(r *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(line)+len(chunk) > limit {
			return nil, errMessageTooLarge
		}
		line = append(line, chunk...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

func serve(ctx context.Context, methods []string, call CallFunc, c *childCall) reply {
	if !slices.Contains(methods, c.Name) {
		return reply{Error: "unknown method: " + c.Name}
	}
	result, err := call(ctx, c.Name, c.Args)
	if err != nil {
		return reply{Error: err.Error()}
	}
	b, err := json.Marshal(result)
	if err != nil {
		return reply{Error: "encode result: " + err.Error()}
	}
	return reply{Result: b}
}

func childFailure(ctx context.Context, cmd *exec.Cmd, stderr *limitedBuffer, timeout time.Duration) error {
	_ = cmd.Wait()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("the script timed out after %s", timeout)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if reason := crashReason(stderr.String()); reason != "" {
		return fmt.Errorf("%s: %s", crashedError, reason)
	}
	return errors.New(crashedError)
}

func crashReason(stderr string) string {
	var first string
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "fatal error:") || strings.HasPrefix(line, "panic:") {
			return line
		}
		if first == "" {
			first = line
		}
	}
	return first
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
