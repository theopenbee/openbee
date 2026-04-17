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

// Invoker spawns kimi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary string
}

func NewInvoker(binary string) *Invoker {
	return &Invoker{binary: binary}
}

// kimiMessage represents a single message in kimi's stream-json output.
type kimiMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content,omitempty"`
}

// ExtractResultFromLog scans a kimi stream-json log file and returns the content
// of the last assistant message, or "" if none found.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastContent string
	ai.ScanJSONLines(f, func(line string) bool {
		var msg kimiMessage
		if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != "assistant" {
			return true
		}
		var s string
		if json.Unmarshal(msg.Content, &s) == nil && s != "" {
			lastContent = s
		}
		return true
	})
	return lastContent
}

// Run starts a kimi CLI process, writing filtered output to logPath.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	args := []string{
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
	if opts.SessionID != "" {
		args = append(args, "--session="+opts.SessionID)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start kimi: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()

		newline := []byte{'\n'}
		var writeErr error
		ai.ScanJSONLines(stdoutPipe, func(line string) bool {
			if _, err := logFile.Write([]byte(line)); err != nil {
				writeErr = err
				return false
			}
			if _, err := logFile.Write(newline); err != nil {
				writeErr = err
				return false
			}
			return true
		})

		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
			return
		}
		if writeErr != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: fmt.Sprintf("write log: %v", writeErr)}
			return
		}
		ch <- ai.Output{Type: ai.OutputDone}
	}()

	return proc, ch, nil
}
