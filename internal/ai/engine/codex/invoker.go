package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "codex"))

const (
	codexEventThreadStarted = "thread.started"
	codexEventItemCompleted = "item.completed"
	codexItemAgentMessage   = "agent_message"
)

// switchableWriter writes to main always, and to branch when set. branch can
// be detached at runtime so subsequent writes go only to main.
type switchableWriter struct {
	mu     sync.Mutex
	main   io.Writer
	branch io.Writer
}

func (s *switchableWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	branch := s.branch
	s.mu.Unlock()
	n, err := s.main.Write(p)
	if branch != nil && n > 0 {
		_, _ = branch.Write(p[:n])
	}
	return n, err
}

func (s *switchableWriter) Detach() {
	s.mu.Lock()
	s.branch = nil
	s.mu.Unlock()
}

// Invoker spawns Codex CLI processes and is safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
	store   *SessionStore
}

// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment at lowest priority.
// OPENBEE_URL is inherited from the server process environment.
func NewInvoker(binary string, store *SessionStore, extraEnv map[string]string) *Invoker {
	return &Invoker{binary: binary, baseEnv: core.NewBaseEnv(extraEnv), store: store}
}

type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item,omitempty"`
}

type codexItem struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Command          string `json:"command,omitempty"`
	AggregatedOutput string `json:"aggregated_output,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Status           string `json:"status,omitempty"`
}

func buildArgs(threadID string, resume bool, prompt string, extraArgs []string) []string {
	var base []string
	if resume && threadID != "" {
		base = []string{"exec", "resume", threadID, "--json", "--dangerously-bypass-approvals-and-sandbox"}
		if prompt != "" {
			base = append(base, prompt)
		}
	} else {
		base = []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	}
	return append(base, extraArgs...)
}

// extractSessionID reads a Codex JSON stream and returns the thread_id from
// the first "thread.started" event, or "" if not found.
func extractSessionID(r io.Reader) string {
	var threadID string
	core.ScanJSONLines(r, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventThreadStarted && event.ThreadID != "" {
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
	core.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventItemCompleted && event.Item != nil &&
			event.Item.Type == codexItemAgentMessage && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
	return lastText
}

// Run starts a Codex CLI process, redirecting output to logPath.
// For new sessions, prompt is passed via stdin; for resumes, the thread_id is resolved from the store.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	threadID, resume := inv.resolveThread(opts.SessionID, opts.Resume)
	args := buildArgs(threadID, resume, prompt, opts.ExtraArgs)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	pr, pw := io.Pipe()
	writer := &switchableWriter{main: logFile, branch: pw}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = core.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey)

	if !resume {
		cmd.Stdin = strings.NewReader(prompt)
	}
	core.ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := core.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
			writer.Detach()
			pw.Close()
		}()

		if !resume {
			if newThreadID := extractSessionID(pr); newThreadID != "" {
				if err := inv.store.Set(opts.SessionID, newThreadID); err != nil {
					log.Error("store codex session", zap.String("uuid", opts.SessionID), zap.Error(err))
				}
				writer.Detach() // stop branching to pipe — pipe writes are no-ops now
			}
		}
		io.Copy(io.Discard, pr) // drain any bytes still in the pipe before pw closes

		if err := <-doneCh; err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}

func (inv *Invoker) resolveThread(openbeeUUID string, resume bool) (threadID string, resolvedResume bool) {
	if !resume {
		return "", false
	}
	threadID, ok := inv.store.Get(openbeeUUID)
	if !ok {
		log.Warn("codex session mapping not found, starting new session", zap.String("uuid", openbeeUUID))
		return "", false
	}
	return threadID, true
}
