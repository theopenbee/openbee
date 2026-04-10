package pi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns pi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary   string
	baseEnv  []string
	extraEnv map[string]string
}

// NewInvoker creates an Invoker. extraEnv keys are injected if non-empty.
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

// buildArgs constructs the pi CLI arguments.
func buildArgs(prompt, sessionPath string) []string {
	return []string{"--mode", "json", "--session", sessionPath, "-p", prompt}
}

// ExtractResultFromLog scans logPath in reverse for the last agent_end event and
// returns the text of the last assistant message's first text content item, or "".
func ExtractResultFromLog(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	lines := bytes.Split(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var event piAgentEnd
		if json.Unmarshal(line, &event) != nil || event.Type != "agent_end" {
			continue
		}
		for j := len(event.Messages) - 1; j >= 0; j-- {
			msg := event.Messages[j]
			if msg.Role != "assistant" {
				continue
			}
			for _, c := range msg.Content {
				if c.Type == "text" && c.Text != "" {
					return c.Text
				}
			}
		}
	}
	return ""
}

// sessionDir returns ~/.openbee/.pi/sessions.
func sessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", ".pi", "sessions"), nil
}

// newSessionPath creates a new unique session file path inside sessionDir.
func newSessionPath() (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir session dir: %w", err)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return filepath.Join(dir, hex.EncodeToString(b)+".jsonl"), nil
}

// buildEnv assembles the subprocess environment.
func (inv *Invoker) buildEnv(apiKey string) []string {
	env := make([]string, len(inv.baseEnv), len(inv.baseEnv)+1+len(inv.extraEnv))
	copy(env, inv.baseEnv)
	env = append(env, "OPENBEE_API_KEY="+apiKey)
	for k, v := range inv.extraEnv {
		if v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env[:len(env):len(env)]
}

// Run is implemented in Task 4.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return nil, nil, fmt.Errorf("not implemented")
}
