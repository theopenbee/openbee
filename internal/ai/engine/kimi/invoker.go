package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

const (
	kimiRoleAssistant = "assistant"
	kimiToolShell     = "Shell"
	kimiContentText   = "text"
	kimiEmptyPrefix   = "(Empty response:"
)

// Invoker spawns Kimi CLI processes. It is immutable after construction and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. extraEnv entries are merged into the base environment (e.g. MOONSHOT_API_KEY).
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
	return &Invoker{binary: binary, baseEnv: core.NewBaseEnv(extraEnv)}
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

// Extractor reads the result text from a Kimi stream-json log.
//
// The content field may be a plain string or an array of content blocks.
// Text blocks starting with "(Empty response:" are skipped — when Kimi ends
// with such a placeholder, the actual response was already sent to the user
// via `openbee ctl message send --stdin`. In that case the heredoc body from
// the last matching Shell tool call is returned instead.
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText, lastSentMsg string
	core.ScanJSONLines(f, func(line string) bool {
		var msg kimiMessage
		if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != kimiRoleAssistant {
			return true
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == kimiToolShell {
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
		trimmed := bytes.TrimSpace(msg.Content)
		if len(trimmed) == 0 {
			return true
		}
		switch trimmed[0] {
		case '"':
			var s string
			if json.Unmarshal(msg.Content, &s) == nil && s != "" && !strings.HasPrefix(s, kimiEmptyPrefix) {
				lastText = s
			}
		case '[':
			var blocks []kimiContentBlock
			if json.Unmarshal(msg.Content, &blocks) != nil {
				return true
			}
			for _, b := range blocks {
				if b.Type == kimiContentText && b.Text != "" && !strings.HasPrefix(b.Text, kimiEmptyPrefix) {
					lastText = b.Text
					break
				}
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

	spec := core.SubprocessSpec{
		Binary:  inv.binary,
		Args:    buildArgs(opts.SessionID, opts.ExtraArgs),
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
	}
	return core.SpawnSubprocess(ctx, spec)
}
