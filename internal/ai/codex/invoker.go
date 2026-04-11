package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Codex CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	return &Invoker{binary: binary, baseEnv: ai.BuildBaseEnv(openbeeURL)}
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

func buildArgs(sessionID string, resume bool, prompt string) []string {
	if resume && sessionID != "" {
		args := []string{"exec", "resume", sessionID, "--json", "--dangerously-bypass-approvals-and-sandbox"}
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
	var threadID string
	ai.ScanJSONLines(r, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			threadID = event.ThreadID
			return false
		}
		return true
	})
	return threadID
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
	ai.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "item.completed" && event.Item != nil &&
			event.Item.Type == "agent_message" && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
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

	pr, pw := io.Pipe()
	writer := io.MultiWriter(logFile, pw)

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	if !opts.Resume {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		// Close pw after cmd.Wait so the pipe reader sees EOF.
		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
			pw.Close()
		}()

		sessionID := extractSessionID(pr)
		if sessionID != "" {
			ch <- ai.Output{Type: ai.OutputSessionID, Content: sessionID}
		}
		io.Copy(io.Discard, pr)

		if err := <-doneCh; err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}
