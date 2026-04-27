package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Claude CLI processes. It is immutable after construction and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment at lowest priority.
// OPENBEE_URL is inherited from the server process environment.
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
}

type streamEvent struct {
	Type    string         `json:"type"`
	Message *streamMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// scanResultLog scans logPath for the terminal result event and the last
// assistant text before it. The result event is always the last event in the
// stream, so scanning stops immediately on hitting it.
func scanResultLog(logPath string) (result string, isError bool, lastAssistantText string) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()
	ai.ScanJSONLines(f, func(line string) bool {
		var event streamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		switch event.Type {
		case "assistant":
			if event.Message != nil && len(event.Message.Content) > 0 {
				if event.Message.Content[0].Type == "text" && event.Message.Content[0].Text != "" {
					lastAssistantText = event.Message.Content[0].Text
				}
			}
		case "result":
			result = event.Result
			isError = event.IsError
			return false
		}
		return true
	})
	return
}

// ExtractResultFromLog scans a Claude stream-json log file and returns the best
// result string: prefers {"type":"result"} over the last assistant text.
func ExtractResultFromLog(logPath string) string {
	result, _, lastAssistantText := scanResultLog(logPath)
	if result != "" {
		return result
	}
	return lastAssistantText
}

// Run starts a Claude CLI process, redirecting output to logPath.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
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
	args = append(args, opts.ExtraArgs...)
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
	cmd.Env = ai.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start claude: %w", err)
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
		result, isError, _ := scanResultLog(logPath)
		if isError {
			if result == "" {
				result = "bee execution failed (no details available)"
			}
			ch <- ai.Output{Type: ai.OutputError, Content: result}
			return
		}
		ch <- ai.Output{Type: ai.OutputDone}
	}()

	return proc, ch, nil
}
