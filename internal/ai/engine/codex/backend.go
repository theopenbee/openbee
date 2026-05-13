package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
	cliargs "github.com/theopenbee/openbee/internal/ai/cliargs"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "codex"))

const (
	codexEventThreadStarted = "thread.started"
	codexEventItemCompleted = "item.completed"
	codexItemAgentMessage   = "agent_message"
)

// switchableWriter writes to main always, and to branch when set. branch can
// be detached at runtime so subsequent writes go only to main.
type switchableWriter struct {
	mu     sync.Mutex
	main   io.Writer
	branch io.Writer
}

func (s *switchableWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	branch := s.branch
	s.mu.Unlock()
	n, err := s.main.Write(p)
	if branch != nil && n > 0 {
		_, _ = branch.Write(p[:n])
	}
	return n, err
}

func (s *switchableWriter) Detach() {
	s.mu.Lock()
	s.branch = nil
	s.mu.Unlock()
}

type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item,omitempty"`
}

type codexItem struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Command          string `json:"command,omitempty"`
	AggregatedOutput string `json:"aggregated_output,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Status           string `json:"status,omitempty"`
}

// Backend is the codex engine implementation of core.BaseAdapter. It spawns the
// Codex CLI process, extracts the final agent_message from the JSON log, and
// reads token-usage data from the session JSONL written by the CLI.
type Backend struct {
	binary     string
	baseEnv    []string
	store      *SessionStore
	mappingDir string
	codexBase  string
}

// NewBackend builds a codex Backend with default mapping/codex roots.
// extraEnv entries are merged into the base environment at lowest priority.
// OPENBEE_URL is inherited from the server process environment.
func NewBackend(binary string, store *SessionStore, extraEnv map[string]string) *Backend {
	return &Backend{
		binary:     binary,
		baseEnv:    core.NewBaseEnv(extraEnv),
		store:      store,
		mappingDir: config.DefaultCodexSessionsDir(),
		codexBase:  config.EngineSessionsDir("CODEX_HOME", defaultCodexBase),
	}
}

// NewBackendAt is a test seam allowing arbitrary mapping/codex roots.
func NewBackendAt(binary string, store *SessionStore, extraEnv map[string]string,
	mappingDir, codexBase string) *Backend {
	return &Backend{
		binary:     binary,
		baseEnv:    core.NewBaseEnv(extraEnv),
		store:      store,
		mappingDir: mappingDir,
		codexBase:  codexBase,
	}
}

func defaultCodexBase() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func buildArgs(threadID string, resume bool, prompt string, extraArgs []string) []string {
	var base []string
	if resume && threadID != "" {
		base = []string{"exec", "resume", threadID, "--json", "--dangerously-bypass-approvals-and-sandbox"}
		if prompt != "" {
			base = append(base, prompt)
		}
	} else {
		base = []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	}
	return append(base, extraArgs...)
}

// extractSessionID reads a Codex JSON stream and returns the thread_id from
// the first "thread.started" event, or "" if not found.
func extractSessionID(r io.Reader) string {
	var threadID string
	core.ScanJSONLines(r, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventThreadStarted && event.ThreadID != "" {
			threadID = event.ThreadID
			return false
		}
		return true
	})
	return threadID
}

// Extract returns the text of the last agent_message item from the Codex JSON log.
func (b *Backend) Extract(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	core.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventItemCompleted && event.Item != nil &&
			event.Item.Type == codexItemAgentMessage && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
	return lastText
}

// Run starts a Codex CLI process, redirecting output to logPath.
// For new sessions, prompt is passed via stdin; for resumes, the thread_id is resolved from the store.
func (b *Backend) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	threadID, resume := b.resolveThread(opts.SessionID, opts.Resume)
	extra, err := cliargs.SplitArgs(opts.ExtraArgs)
	if err != nil {
		return nil, nil, fmt.Errorf("parse extra args: %w", err)
	}
	args := buildArgs(threadID, resume, prompt, extra)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	pr, pw := io.Pipe()
	writer := &switchableWriter{main: logFile, branch: pw}

	cmd := exec.CommandContext(ctx, b.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = core.BuildRunEnv(b.baseEnv, opts.ExtraEnv, opts.APIKey)

	if !resume {
		cmd.Stdin = strings.NewReader(prompt)
	}
	core.ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := core.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
			writer.Detach()
			pw.Close()
		}()

		if !resume {
			if newThreadID := extractSessionID(pr); newThreadID != "" {
				if err := b.store.Set(opts.SessionID, newThreadID); err != nil {
					log.Error("store codex session", zap.String("uuid", opts.SessionID), zap.Error(err))
				}
				writer.Detach() // stop branching to pipe — pipe writes are no-ops now
			}
		}
		io.Copy(io.Discard, pr) // drain any bytes still in the pipe before pw closes

		if err := <-doneCh; err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
	}()

	return proc, ch, nil
}

func (b *Backend) resolveThread(openbeeUUID string, resume bool) (threadID string, resolvedResume bool) {
	if !resume {
		return "", false
	}
	threadID, ok := b.store.Get(openbeeUUID)
	if !ok {
		log.Warn("codex session mapping not found, starting new session", zap.String("uuid", openbeeUUID))
		return "", false
	}
	return threadID, true
}

const (
	codexLineTurnContext = "turn_context"
	codexLineEventMsg    = "event_msg"
	codexPayloadTokens   = "token_count"
)

type codexJSONLLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type  string          `json:"type"`
		Model string          `json:"model"`
		Info  *codexTokenInfo `json:"info"`
	} `json:"payload"`
	Info *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	Model     string `json:"model"`
	ModelName string `json:"model_name"`
	Metadata  struct {
		Model string `json:"model"`
	} `json:"metadata"`
	LastTokenUsage  *codexTokenUsage `json:"last_token_usage"`
	TotalTokenUsage *codexTokenUsage `json:"total_token_usage"`
}

type codexTokenUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
}

func (t *codexTokenUsage) advance(usage codexTokenUsage) {
	t.InputTokens += usage.InputTokens
	t.OutputTokens += usage.OutputTokens
	t.CachedInputTokens += usage.CachedInputTokens
}

func (t *codexTokenUsage) deltaAndSet(total codexTokenUsage) codexTokenUsage {
	delta := codexTokenUsage{
		InputTokens:       total.InputTokens - t.InputTokens,
		OutputTokens:      total.OutputTokens - t.OutputTokens,
		CachedInputTokens: total.CachedInputTokens - t.CachedInputTokens,
	}
	*t = total
	return delta
}

func (l codexJSONLLine) tokenInfo() *codexTokenInfo {
	if l.Payload.Info != nil {
		return l.Payload.Info
	}
	return l.Info
}

// Collect reads the openbee-UUID → codex-thread-ID mapping and aggregates token usage.
func (b *Backend) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(b.mappingDir, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: read codex mapping for session %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path, err := findCodexSessionFile(b.codexBase, codexSessionID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: codex session file not found for %s", ai.ErrSessionDataNotFound, codexSessionID)
		}
		return nil, err
	}
	return parseCodexFile(path)
}

func findCodexSessionFile(codexBase, sessionID string) (string, error) {
	return sessionfile.FindWithLegacyFast(
		filepath.Join(codexBase, "sessions"),
		sessionID+".jsonl",
		func(_ string, d os.DirEntry) bool {
			return strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), sessionID)
		},
	)
}

func parseCodexFile(path string) ([]ai.TokenUsage, error) {
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	usages, err := core.AggregateUsage[codexJSONLLine](path, func(line codexJSONLLine, agg map[string]*ai.TokenUsage) {
		switch line.Type {
		case codexLineTurnContext:
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case codexLineEventMsg:
			if line.Payload.Type != "" && line.Payload.Type != codexPayloadTokens {
				return
			}
			info := line.tokenInfo()
			if info == nil {
				return
			}
			m := codexResolveModel(info, currentModel)
			if m == "" {
				return
			}
			u := agg[m]
			if u == nil {
				u = &ai.TokenUsage{Model: m}
				agg[m] = u
			}
			if prevByModel[m] == nil {
				prevByModel[m] = &codexTokenUsage{}
			}
			prev := prevByModel[m]
			if info.LastTokenUsage != nil {
				addCodexUsage(u, *info.LastTokenUsage)
				if info.TotalTokenUsage != nil {
					// Codex emits both fields together when a turn is replayed/resumed;
					// the cumulative total is authoritative, so reset prev instead of
					// double-counting by advancing it.
					*prev = *info.TotalTokenUsage
				} else {
					prev.advance(*info.LastTokenUsage)
				}
			} else if info.TotalTokenUsage != nil {
				addCodexUsage(u, prev.deltaAndSet(*info.TotalTokenUsage))
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return usages, nil
}

func addCodexUsage(dst *ai.TokenUsage, usage codexTokenUsage) {
	dst.InputTokens += usage.InputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheReadTokens += usage.CachedInputTokens
}

func codexResolveModel(info *codexTokenInfo, currentModel string) string {
	if info.Model != "" {
		return info.Model
	}
	if info.ModelName != "" {
		return info.ModelName
	}
	if info.Metadata.Model != "" {
		return info.Metadata.Model
	}
	return currentModel
}
