package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns pi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary     string
	baseEnv    []string // pre-built env (openbee vars + extraEnv), without per-run API key
	sessionDir string   // ~/.openbee/.pi/sessions, created once at startup
}

func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) (*Invoker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}
	sessionDir := filepath.Join(home, ".openbee", ".pi", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}
	base := ai.BuildBaseEnv(openbeeURL)
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	return &Invoker{binary: binary, baseEnv: base, sessionDir: sessionDir}, nil
}

type piAgentEnd struct {
	Type     string      `json:"type"`
	Messages []piMessage `json:"messages"`
}

type piMessage struct {
	Role    string      `json:"role"`
	Content []piContent `json:"content"`
}

type piContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildArgs(prompt, sessionPath string) []string {
	return []string{"--mode", "json", "--session", sessionPath, "-p", prompt}
}

// ExtractResultFromLog scans logPath for the last agent_end event and returns
// the text of the last assistant message's first text content item, or "".
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event piAgentEnd
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != "agent_end" {
			continue
		}
		for j := len(event.Messages) - 1; j >= 0; j-- {
			msg := event.Messages[j]
			if msg.Role != "assistant" {
				continue
			}
			for _, c := range msg.Content {
				if c.Type == "text" && c.Text != "" {
					lastText = c.Text
					goto nextLine
				}
			}
		}
	nextLine:
	}
	return lastText
}

func (inv *Invoker) sessionFilePath(sessionID string) string {
	return filepath.Join(inv.sessionDir, sessionID+".jsonl")
}

// Run starts a pi CLI process, redirecting stdout+stderr to logPath.
// opts.SessionID must be a UUID; the session file path is derived as
// ~/.openbee/.pi/sessions/{sessionID}.jsonl. Resume vs. new session is
// inferred by pi CLI from whether that file already exists.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	sessionPath := inv.sessionFilePath(opts.SessionID)

	args := buildArgs(prompt, sessionPath)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(append([]string{}, inv.baseEnv...), "OPENBEE_API_KEY="+opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start pi: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}
