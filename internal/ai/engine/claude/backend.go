package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

const (
	// systemRulesFile is the legacy rules file Claude's Run cleanup removes.
	systemRulesFile = ".openbee.md"
	// importLine is the legacy reference line removed from CLAUDE.md.
	importLine = "@" + systemRulesFile

	syntheticModel = "<synthetic>"
)

// Backend is the claude engine implementation of core.BaseAdapter. It spawns
// the Claude CLI process, extracts the final result from the stream-json log,
// and reads token-usage data from the session JSONL written by the CLI.
type Backend struct {
	binary   string
	baseEnv  []string // pre-built env (openbee vars + extraEnv), without per-run API key
	baseDirs []string // directories Collect searches for session files (honors CLAUDE_CONFIG_DIR)
}

// NewBackend creates a Backend. extraEnv entries are merged into the base
// environment at lowest priority. Collect honors CLAUDE_CONFIG_DIR if set,
// falling back to ~/.claude and ~/.config/claude.
func NewBackend(binary string, extraEnv map[string]string) *Backend {
	return &Backend{
		binary:   binary,
		baseEnv:  core.NewBaseEnv(extraEnv),
		baseDirs: claudeBaseDirs(),
	}
}

func claudeBaseDirs() []string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		return utils.SplitAndTrim(env)
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "claude"),
	}
}

// --- Stream parsing types ---

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

// Extract reads the result text from a Claude stream-json log file: prefers
// {"type":"result"} over the last assistant text.
func (b *Backend) Extract(logPath string) string {
	result, _, lastAssistantText := scanResultLog(logPath)
	if result != "" {
		return result
	}
	return lastAssistantText
}

// Run cleans up legacy openbee rules before launching the Claude CLI process.
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	if err := cleanupLegacyRules(workDir); err != nil {
		return nil, nil, err
	}
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
		Binary:  b.binary,
		Args:    args,
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(b.baseEnv, opts.ExtraEnv, opts.APIKey),
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

// --- Legacy cleanup ---

func cleanupLegacyRules(workDir string) error {
	rulesPath := filepath.Join(workDir, systemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", systemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(importLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}

// --- Token usage (Collect) ---

type claudeJSONLLine struct {
	Message struct {
		Model string `json:"model"`
		Speed string `json:"speed"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Collect reads token usage for the given session from the claude session JSONL file.
func (b *Backend) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	name := sessionID + ".jsonl"
	for _, base := range b.baseDirs {
		path, err := sessionfile.FindWithLegacyFast(
			filepath.Join(base, "projects"),
			name,
			func(_ string, d os.DirEntry) bool { return d.Name() == name },
		)
		if err == nil {
			return parseClaudeFile(path)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: claude session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
}

func parseClaudeFile(path string) ([]ai.TokenUsage, error) {
	usages, err := core.AggregateUsage[claudeJSONLLine](path, func(line claudeJSONLLine, agg map[string]*ai.TokenUsage) {
		m := line.Message.Model
		if m == "" || m == syntheticModel || line.Message.Usage == nil {
			return
		}
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u := agg[m]
		if u == nil {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	})
	if err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return usages, nil
}
