package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// OutputType classifies a line of output from a Claude CLI process.
type OutputType string

const (
	OutputStdout OutputType = "stdout"
	OutputStderr OutputType = "stderr"
	OutputDone   OutputType = "done"
	OutputError  OutputType = "error"
)

// Output is a single output event from a Claude CLI process.
type Output struct {
	Type    OutputType `json:"type"`
	Content string     `json:"content"`
}

// RunOptions controls session behaviour for a Claude CLI invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
}

// Invoker spawns Claude CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is the openbee server base URL
// (e.g. "http://host:port") injected as OPENBEE_URL into the subprocess.
func NewInvoker(binary, openbeeURL string) *Invoker {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+3)
	if exePath, err := os.Executable(); err == nil {
		patchedPath := "PATH=" + filepath.Dir(exePath) + string(os.PathListSeparator) + os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, patchedPath)
	} else {
		env = append(env, sysEnv...)
	}
	env = append(env, "OPENBEE_URL="+openbeeURL)
	return &Invoker{binary: binary, baseEnv: env}
}

// Process represents a running Claude CLI invocation.
type Process struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}

// PID returns the process ID, or 0 if the process has not started.
func (p *Process) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Stop kills the process.
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// Run starts a Claude CLI process, redirecting its stdout and stderr to logPath.
// The returned channel carries only lifecycle events: OutputDone on success,
// OutputError on failure. The channel is closed after the process exits.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (*Process, <-chan Output, error) {
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, "--print")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()

		if err := cmd.Wait(); err != nil {
			ch <- Output{Type: OutputError, Content: err.Error()}
		} else {
			ch <- Output{Type: OutputDone, Content: ""}
		}
	}()

	return proc, ch, nil
}
