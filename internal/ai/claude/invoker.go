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

// Invoker spawns Claude CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	return &Invoker{binary: binary, baseEnv: ai.BuildBaseEnv(openbeeURL)}
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

// ExtractResultFromLog scans a Claude stream-json log file and returns the best
// result string: prefers {"type":"result"} over the last assistant text.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastAssistantText, streamResult string
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
			if event.Result != "" {
				streamResult = event.Result
			}
		}
		return true
	})
	if streamResult != "" {
		return streamResult
	}
	return lastAssistantText
}

// extractResultStatus scans a Claude stream-json log file and returns the
// result string and whether is_error was true in the result event.
func extractResultStatus(logPath string) (result string, isError bool) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", false
	}
	defer f.Close()
	ai.ScanJSONLines(f, func(line string) bool {
		var event streamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "result" {
			result = event.Result
			isError = event.IsError
			return false // result event is terminal; stop scanning
		}
		return true
	})
	return
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
