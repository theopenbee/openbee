package claude

import (
	"context"
	"encoding/json"
	"os"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

// Invoker spawns Claude CLI processes. It is immutable after construction and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment at lowest priority.
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	return &Invoker{binary: binary, baseEnv: core.NewBaseEnv(extraEnv)}
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
	core.ScanJSONLines(f, func(line string) bool {
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

// Extractor reads the result text from a Claude stream-json log file: prefers
// {"type":"result"} over the last assistant text.
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
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

	spec := core.SubprocessSpec{
		Binary:  inv.binary,
		Args:    args,
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
		PostWait: func(waitErr error, logPath string) *ai.Output {
			if waitErr != nil {
				return nil // default mapping → OutputError(waitErr.Error())
			}
			result, isError, _ := scanResultLog(logPath)
			if !isError {
				return nil // default → OutputDone
			}
			if result == "" {
				result = "bee execution failed (no details available)"
			}
			return &ai.Output{Type: ai.OutputError, Content: result}
		},
	}
	return core.SpawnSubprocess(ctx, spec)
}
