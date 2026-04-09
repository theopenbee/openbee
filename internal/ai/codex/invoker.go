package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Codex CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+2)
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

// Process represents a running Codex CLI invocation.
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

type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item,omitempty"`
}

type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// buildArgs constructs the codex CLI arguments.
func buildArgs(sessionID string, resume bool, prompt string) []string {
	if resume && sessionID != "" {
		args := []string{"exec", "resume", sessionID, "--json", "--yolo"}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args
	}
	return []string{"exec", "-", "--json", "--yolo"}
}

// extractSessionID reads a Codex JSON stream and returns the thread_id from
// the first "thread.started" event, or "" if not found.
func extractSessionID(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			return event.ThreadID
		}
	}
	return ""
}

// ExtractResultFromLog scans a Codex JSON log file and returns the text of the
// last agent_message item, or "" if none found.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "item.completed" && event.Item != nil &&
			event.Item.Type == "agent_message" && event.Item.Text != "" {
			lastText = event.Item.Text
		}
	}
	return lastText
}

// Run starts a Codex CLI process, redirecting output to logPath.
// For new sessions (Resume=false), prompt is passed via stdin.
// For resume sessions, prompt is passed as a follow-up argument (if non-empty).
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	args := buildArgs(opts.SessionID, opts.Resume, prompt)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	// Tee stdout to both the log file and an in-memory pipe for session ID extraction.
	pr, pw := io.Pipe()
	writer := io.MultiWriter(logFile, pw)

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	// For new sessions, pass prompt via stdin.
	if !opts.Resume {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		// Read the pipe to extract session ID, then drain remaining output.
		sessionID := extractSessionID(pr)
		if sessionID != "" {
			ch <- ai.Output{Type: ai.OutputSessionID, Content: sessionID}
		}
		// Drain the rest so the writer doesn't block.
		io.Copy(io.Discard, pr)

		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
		pw.Close()
	}()

	return proc, ch, nil
}
