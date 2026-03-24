// internal/claude/invoker.go
package claude

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os/exec"
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
}

// Invoker spawns Claude CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary string
	mcpURL string
	apiKey string
}

// NewInvoker creates an Invoker. mcpBasePath should include the full MCP base
// path (e.g. "http://host:port/mcp/bee"); "/sse" is appended automatically.
func NewInvoker(binary, mcpBasePath, apiKey string) *Invoker {
	return &Invoker{
		binary: binary,
		mcpURL: mcpBasePath + "/sse",
		apiKey: apiKey,
	}
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

// Run starts a Claude CLI process and returns a Process handle and an output channel.
// The channel is closed after the process exits; the last message is OutputDone or OutputError.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions) (*Process, <-chan Output, error) {
	mcpConfig := fmt.Sprintf(
		`{"mcpServers":{"openbee":{"type":"sse","url":%q}}}`,
		inv.mcpURL+"?api_key="+url.QueryEscape(inv.apiKey),
	)
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--mcp-config", mcpConfig,
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, "--print")

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan Output, 100)

	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				ch <- Output{Type: OutputStdout, Content: scanner.Text()}
			}
		}()

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				ch <- Output{Type: OutputStderr, Content: scanner.Text()}
			}
		}()

		wg.Wait()

		if err := cmd.Wait(); err != nil {
			ch <- Output{Type: OutputError, Content: err.Error()}
		} else {
			ch <- Output{Type: OutputDone, Content: ""}
		}
	}()

	return proc, ch, nil
}
