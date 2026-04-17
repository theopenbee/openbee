package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Kimi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
// extraEnv entries are merged into the base environment (e.g. MOONSHOT_API_KEY).
func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv(openbeeURL)
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	base = base[:len(base):len(base)]
	return &Invoker{binary: binary, baseEnv: base}
}

func buildArgs(sessionID string) []string {
	return []string{
		"--session=" + sessionID,
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
}

type kimiMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type kimiContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ExtractResultFromLog scans a Kimi stream-json log and returns the text of the
// last role=assistant message, or "" if none found.
// The content field may be a plain string or an array of content blocks.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	ai.ScanJSONLines(f, func(line string) bool {
		var msg kimiMessage
		if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != "assistant" {
			return true
		}
		if len(msg.Content) == 0 {
			return true
		}
		// Try string content first.
		var s string
		if json.Unmarshal(msg.Content, &s) == nil && s != "" {
			lastText = s
			return true
		}
		// Try array of content blocks.
		var blocks []kimiContentBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return true
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				lastText = b.Text
				break
			}
		}
		return true
	})
	return lastText
}

// Run starts a Kimi CLI process, redirecting output to logPath.
// The prompt is passed via stdin; opts.SessionID is passed as --session=<UUID>.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	args := buildArgs(opts.SessionID)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = ai.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start kimi: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
			return
		}
		ch <- ai.Output{Type: ai.OutputDone}
	}()

	return proc, ch, nil
}
