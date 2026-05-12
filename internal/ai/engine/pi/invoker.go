package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// Invoker spawns pi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary     string
	baseEnv    []string // pre-built env (openbee vars + extraEnv), without per-run API key
	sessionDir string   // ~/.openbee/.pi/sessions, created once at startup
}

func NewInvoker(binary string, extraEnv map[string]string) (*Invoker, error) {
	sessionDir := config.DefaultPiSessionsDir()
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir session dir: %w", err)
	}
	return &Invoker{binary: binary, baseEnv: core.NewBaseEnv(extraEnv), sessionDir: sessionDir}, nil
}

type piAgentEnd struct {
	Type     string      `json:"type"`
	Messages []piMessage `json:"messages"`
}

type piMessage struct {
	Role         string      `json:"role"`
	Content      []piContent `json:"content"`
	StopReason   string      `json:"stopReason,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
}

type piContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

const (
	eventTypeAgentEnd = "agent_end"
	roleAssistant     = "assistant"
	stopReasonError   = "error"
	contentTypeText   = "text"
)

func buildArgs(prompt, sessionPath string, extraArgs []string) []string {
	base := []string{"--mode", "json", "--session", sessionPath, "-p", prompt}
	return append(base, extraArgs...)
}

func scanLastAssistantMessage(logPath string) *piMessage {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var result *piMessage
	core.ScanJSONLines(f, func(line string) bool {
		var event piAgentEnd
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != eventTypeAgentEnd {
			return true
		}
		for j := len(event.Messages) - 1; j >= 0; j-- {
			msg := event.Messages[j]
			if msg.Role == roleAssistant {
				result = &msg
				return true
			}
		}
		return true
	})
	return result
}

// ExtractResultFromLog scans logPath for the last agent_end event and returns
// the text of the last assistant message's first text content item, or "".
func ExtractResultFromLog(logPath string) string {
	msg := scanLastAssistantMessage(logPath)
	if msg == nil {
		return ""
	}
	for _, c := range msg.Content {
		if c.Type == contentTypeText && c.Text != "" {
			return c.Text
		}
	}
	return ""
}

// checkAgentError detects cases where pi exits cleanly (exit 0) but the
// underlying API call failed (e.g. 401 authentication error). Returns the
// errorMessage from the last assistant message if stopReason is "error", or "".
func checkAgentError(logPath string) string {
	msg := scanLastAssistantMessage(logPath)
	if msg == nil || msg.StopReason != stopReasonError {
		return ""
	}
	return msg.ErrorMessage
}

// extractPiError returns the first non-JSON line from stderr, or fallback.
// pi emits human-readable error text before any JSON on failure.
func extractPiError(stderr, fallback string) string {
	scanner := bufio.NewScanner(strings.NewReader(stderr))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '{' || line[0] == '[' {
			continue
		}
		return line
	}
	return fallback
}

// limitWriter caps how much of stderr lands in the in-memory buffer so a
// misbehaving process cannot exhaust heap. Writes beyond the cap are dropped
// from the buffer but still flow through to the log file via MultiWriter.
type limitWriter struct {
	w   io.Writer
	rem int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.rem <= 0 {
		return len(p), nil
	}
	orig := len(p)
	if len(p) > l.rem {
		p = p[:l.rem]
	}
	n, err := l.w.Write(p)
	l.rem -= n
	if err != nil {
		return n, err
	}
	return orig, nil
}

func (inv *Invoker) sessionFilePath(sessionID string) string {
	return filepath.Join(inv.sessionDir, sessionID+".jsonl")
}

// Run starts a pi CLI process, redirecting stdout+stderr to logPath.
// opts.SessionID must be a UUID; the session file path is derived as
// ~/.openbee/.pi/sessions/{sessionID}.jsonl. Resume vs. new session is
// inferred by pi CLI from whether that file already exists.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	sessionPath := inv.sessionFilePath(opts.SessionID)

	args := buildArgs(prompt, sessionPath, opts.ExtraArgs)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	var stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stderr = io.MultiWriter(logFile, &limitWriter{w: &stderrBuf, rem: 4096})
	cmd.Env = core.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	core.ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start pi: %w", err)
	}

	proc := core.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()

		var writeErr error
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(nil, 1024*1024)
		newline := []byte{'\n'}
		for scanner.Scan() {
			line := scanner.Bytes()
			if isStreamingDelta(line) {
				continue
			}
			line = stripThinkingSignature(line)
			if _, err := logFile.Write(line); err != nil {
				writeErr = err
				break
			}
			if _, err := logFile.Write(newline); err != nil {
				writeErr = err
				break
			}
		}

		if err := cmd.Wait(); err != nil {
			msg := extractPiError(stderrBuf.String(), err.Error())
			ch <- ai.Output{Type: ai.OutputError, Content: msg}
		} else if writeErr != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: fmt.Sprintf("write log: %v", writeErr)}
		} else if errMsg := checkAgentError(logPath); errMsg != "" {
			ch <- ai.Output{Type: ai.OutputError, Content: errMsg}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}

// stripThinkingSignature removes the thinkingSignature field from thinking
// content blocks in log lines to keep log files compact. It handles both
// message_end events (single "message" object) and agent_end events
// ("messages" array).
func stripThinkingSignature(line []byte) []byte {
	if !bytes.Contains(line, []byte(`"thinkingSignature"`)) {
		return line
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return line
	}

	// Handle message_end: single "message" object.
	if msgRaw, ok := raw["message"]; ok {
		newMsgRaw, changed := stripThinkingSignatureFromMessage(msgRaw)
		if !changed {
			return line
		}
		raw["message"] = newMsgRaw
		result, err := json.Marshal(raw)
		if err != nil {
			return line
		}
		return result
	}

	// Handle agent_end: "messages" array.
	if msgsRaw, ok := raw["messages"]; ok {
		var msgs []json.RawMessage
		if json.Unmarshal(msgsRaw, &msgs) != nil {
			return line
		}
		changed := false
		for i, msgRaw := range msgs {
			newMsgRaw, ok := stripThinkingSignatureFromMessage(msgRaw)
			if ok {
				msgs[i] = newMsgRaw
				changed = true
			}
		}
		if !changed {
			return line
		}
		newMsgs, err := json.Marshal(msgs)
		if err != nil {
			return line
		}
		raw["messages"] = newMsgs
		result, err := json.Marshal(raw)
		if err != nil {
			return line
		}
		return result
	}

	return line
}

// stripThinkingSignatureFromMessage removes thinkingSignature from all content
// blocks within a single message JSON object. Returns the modified message and
// whether any change was made.
func stripThinkingSignatureFromMessage(msgRaw json.RawMessage) (json.RawMessage, bool) {
	var msg map[string]json.RawMessage
	if json.Unmarshal(msgRaw, &msg) != nil {
		return msgRaw, false
	}
	contentRaw, ok := msg["content"]
	if !ok {
		return msgRaw, false
	}
	var content []json.RawMessage
	if json.Unmarshal(contentRaw, &content) != nil {
		return msgRaw, false
	}
	changed := false
	for i, item := range content {
		var block map[string]json.RawMessage
		if json.Unmarshal(item, &block) != nil {
			continue
		}
		if _, has := block["thinkingSignature"]; !has {
			continue
		}
		delete(block, "thinkingSignature")
		newItem, err := json.Marshal(block)
		if err != nil {
			continue
		}
		content[i] = newItem
		changed = true
	}
	if !changed {
		return msgRaw, false
	}
	newContent, err := json.Marshal(content)
	if err != nil {
		return msgRaw, false
	}
	msg["content"] = newContent
	newMsg, err := json.Marshal(msg)
	if err != nil {
		return msgRaw, false
	}
	return newMsg, true
}

// isStreamingDelta reports whether a JSON log line is a streaming delta event
// that should be filtered to keep log files compact. Both message_update and
// tool_execution_update events contain incremental deltas that are superseded
// by the complete content in the corresponding message_end / tool_execution_end event.
func isStreamingDelta(line []byte) bool {
	return bytes.Contains(line, []byte(`"type":"message_update"`)) ||
		bytes.Contains(line, []byte(`"type":"tool_execution_update"`))
}
