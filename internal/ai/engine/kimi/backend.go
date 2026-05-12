package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

const (
	kimiRoleAssistant = "assistant"
	kimiToolShell     = "Shell"
	kimiContentText   = "text"
	kimiEmptyPrefix   = "(Empty response:"
	kimiModel         = "kimi"
)

// Backend is the kimi engine implementation of core.BaseAdapter. It spawns the
// kimi CLI process, extracts the final assistant message from the JSON log, and
// reads token-usage data from the session JSONL written by the CLI.
type Backend struct {
	binary      string
	baseEnv     []string // pre-built env (openbee vars + extraEnv), without per-run API key
	sessionsDir string   // directory Collect searches for session files
}

// NewBackend creates a Backend. extraEnv entries are merged into the base
// environment (e.g. MOONSHOT_API_KEY). sessionsDir defaults to
// config.DefaultKimiSessionsDir() (~/.kimi/sessions).
func NewBackend(binary string, extraEnv map[string]string) *Backend {
	return &Backend{
		binary:      binary,
		baseEnv:     core.NewBaseEnv(extraEnv),
		sessionsDir: config.DefaultKimiSessionsDir(),
	}
}

// NewBackendAt is a test seam allowing an arbitrary sessions root.
// It mirrors the old NewCollectorAt semantics.
func NewBackendAt(binary string, extraEnv map[string]string, sessionsDir string) *Backend {
	return &Backend{
		binary:      binary,
		baseEnv:     core.NewBaseEnv(extraEnv),
		sessionsDir: sessionsDir,
	}
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

// Extract reads the result text from a Kimi stream-json log.
//
// The content field may be a plain string or an array of content blocks.
// Text blocks starting with "(Empty response:" are skipped — when Kimi ends
// with such a placeholder, the actual response was already sent to the user
// via `openbee ctl message send --stdin`. In that case the heredoc body from
// the last matching Shell tool call is returned instead.
func (b *Backend) Extract(logPath string) string {
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
			for _, bl := range blocks {
				if bl.Type == kimiContentText && bl.Text != "" && !strings.HasPrefix(bl.Text, kimiEmptyPrefix) {
					lastText = bl.Text
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
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	spec := core.SubprocessSpec{
		Binary:  b.binary,
		Args:    buildArgs(opts.SessionID, opts.ExtraArgs),
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(b.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
	}
	return core.SpawnSubprocess(ctx, spec)
}

// --- Token usage (Collect) ---

type kimiTokenUsage struct {
	InputOther         int64 `json:"input_other"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"input_cache_read"`
	InputCacheCreation int64 `json:"input_cache_creation"`
}

type kimiJSONLLine struct {
	Message struct {
		Type    string `json:"type"`
		Payload struct {
			TokenUsage *kimiTokenUsage `json:"token_usage"`
		} `json:"payload"`
	} `json:"message"`
}

// Collect reads token usage for the given session from the kimi session JSONL file.
func (b *Backend) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	matches, err := filepath.Glob(filepath.Join(b.sessionsDir, "*", sessionID, "wire.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob kimi session: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: kimi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
	}
	return parseKimiFile(matches[0])
}

func parseKimiFile(path string) ([]ai.TokenUsage, error) {
	var last *kimiTokenUsage
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line kimiJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Message.Type != "StatusUpdate" || line.Message.Payload.TokenUsage == nil {
			return
		}
		last = line.Message.Payload.TokenUsage
	})
	if err != nil {
		return nil, fmt.Errorf("scan kimi session file: %w", err)
	}
	if last == nil {
		return nil, fmt.Errorf("%w: no StatusUpdate found in %s", ai.ErrSessionDataNotFound, path)
	}
	return []ai.TokenUsage{{
		Model:               kimiModel,
		InputTokens:         last.InputOther,
		OutputTokens:        last.Output,
		CacheReadTokens:     last.InputCacheRead,
		CacheCreationTokens: last.InputCacheCreation,
	}}, nil
}
