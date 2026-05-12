package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Kimi CLI processes. It is immutable after construction and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment (e.g. MOONSHOT_API_KEY).
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv()
	return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
}

func buildArgs(sessionID string, extraArgs []string) []string {
	base := []string{
		"--session=" + sessionID,
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
	return append(base, extraArgs...)
}

type kimiToolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type kimiMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []kimiToolCall  `json:"tool_calls"`
}

type kimiContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var heredocRe = regexp.MustCompile(`(?s)<<\s*'?EOF'?\n(.*?)\nEOF`)

// extractSentMsg extracts the heredoc stdin body from an
// `openbee ctl message send --stdin << 'EOF' ... EOF` shell command.
// Returns "" if the command is not applicable or has no heredoc.
func extractSentMsg(command string) string {
	if !strings.Contains(command, "openbee ctl message send") ||
		!strings.Contains(command, "--stdin") {
		return ""
	}
	m := heredocRe.FindStringSubmatch(command)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ExtractResultFromLog scans a Kimi stream-json log and returns the last
// meaningful result text, or "" if none found.
//
// The content field may be a plain string or an array of content blocks.
// Text blocks starting with "(Empty response:" are skipped — when Kimi ends
// with such a placeholder, the actual response was already sent to the user
// via `openbee ctl message send --stdin`. In that case the heredoc body from
// the last matching Shell tool call is returned instead.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText, lastSentMsg string
	ai.ScanJSONLines(f, func(line string) bool {
		var msg kimiMessage
		if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != "assistant" {
			return true
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == "Shell" {
				var args struct {
					Command string `json:"command"`
				}
				if json.Unmarshal([]byte(tc.Function.Arguments), &args) == nil {
					if s := extractSentMsg(args.Command); s != "" {
						lastSentMsg = s
					}
				}
			}
		}
		if len(msg.Content) == 0 {
			return true
		}
		var s string
		if json.Unmarshal(msg.Content, &s) == nil && s != "" {
			if !strings.HasPrefix(s, "(Empty response:") {
				lastText = s
			}
			return true
		}
		var blocks []kimiContentBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return true
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" && !strings.HasPrefix(b.Text, "(Empty response:") {
				lastText = b.Text
				break
			}
		}
		return true
	})
	if lastText != "" {
		return lastText
	}
	return lastSentMsg
}

// Run starts a Kimi CLI process, redirecting output to logPath.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	args := buildArgs(opts.SessionID, opts.ExtraArgs)

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
	ai.ConfigureCmd(cmd)

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
