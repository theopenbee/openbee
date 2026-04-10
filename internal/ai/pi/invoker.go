package pi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	binary   string
	baseEnv  []string
	extraEnv map[string]string
}

func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) *Invoker {
	return &Invoker{
		binary:   binary,
		baseEnv:  ai.BuildBaseEnv(openbeeURL),
		extraEnv: extraEnv,
	}
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

func newSessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".pi", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir session dir: %w", err)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return filepath.Join(dir, hex.EncodeToString(b)+".jsonl"), nil
}

func (inv *Invoker) buildEnv(apiKey string) []string {
	env := make([]string, len(inv.baseEnv), len(inv.baseEnv)+1+len(inv.extraEnv))
	copy(env, inv.baseEnv)
	env = append(env, "OPENBEE_API_KEY="+apiKey)
	for k, v := range inv.extraEnv {
		if v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Run starts a pi CLI process, redirecting stdout+stderr to logPath.
// The session file path is either taken from opts.SessionID (resume) or generated fresh.
// OutputSessionID is emitted before the process starts its goroutine.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	sessionPath := opts.SessionID
	if sessionPath == "" {
		var err error
		sessionPath, err = newSessionPath()
		if err != nil {
			return nil, nil, fmt.Errorf("pi session path: %w", err)
		}
	}

	args := buildArgs(prompt, sessionPath)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = inv.buildEnv(opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start pi: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 2)

	// Emit session ID immediately — path is known before process output.
	ch <- ai.Output{Type: ai.OutputSessionID, Content: sessionPath}

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
