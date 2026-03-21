package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/claudemd"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

var log = logger.With(zap.String("component", "worker"))

type claudeStreamEvent struct {
	Type    string         `json:"type"`
	Message *claudeMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
}

type claudeMessage struct {
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// executionLog is a thread-safe log buffer for in-flight executions.
type executionLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *executionLog) writeLine(s string) {
	l.mu.Lock()
	l.buf.WriteString(s)
	l.buf.WriteByte('\n')
	l.mu.Unlock()
}

func (l *executionLog) string() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type Manager struct {
	workerBaseDir  string
	beeCfg         config.BeeConfig
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	invoker        *claude.Invoker

	activeProcesses map[string]*claude.Process // execution_id -> process
	activeLogs      map[string]*executionLog   // execution_id -> live log buffer
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
) *Manager {
	return &Manager{
		workerBaseDir:   workerBaseDir,
		beeCfg:          bc,
		workerStore:     ws,
		executionStore:  es,
		invoker:         claude.NewInvoker(bc.Claude.Path, bc.MCPBaseURL, bc.MCP.APIKey),
		activeProcesses: make(map[string]*claude.Process),
		activeLogs:      make(map[string]*executionLog),
	}
}

// GetActiveLog returns the current accumulated log content for a running execution.
// Returns ("", false) if the execution is not currently active.
func (m *Manager) GetActiveLog(executionID string) (string, bool) {
	m.mu.RLock()
	el, ok := m.activeLogs[executionID]
	m.mu.RUnlock()
	if !ok {
		return "", false
	}
	return el.string(), true
}

func (m *Manager) CreateWorker(
	name, description, memory string,
	workDir string,
) (model.Worker, error) {
	id := uuid.New().String()
	if workDir == "" {
		workDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	// Initialize CLAUDE.md only if it doesn't already exist
	claudeMD := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		initialContent := claudemd.ImportLine + "\n"
		if err := os.WriteFile(claudeMD, []byte(initialContent), 0644); err != nil {
			return model.Worker{}, fmt.Errorf("create CLAUDE.md: %w", err)
		}
	}

	if err := claudemd.EnsureSystemRules(workDir, claudemd.RoleWorker, claudemd.WithName(name), claudemd.WithDescription(description), claudemd.WithMemory(memory)); err != nil {
		log.Error("ensure system rules", zap.String("op", "create"), zap.Error(err))
	}

	return m.workerStore.Create(model.Worker{
		ID:          id,
		Name:        name,
		Description: description,
		Memory:      memory,
		WorkDir:     workDir,
	})
}

// ExecuteWorker runs a worker. When sessionID is non-empty, it resumes the existing
// Claude session (resume=true); otherwise it starts a fresh session.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := claudemd.EnsureSystemRules(worker.WorkDir, claudemd.RoleWorker, claudemd.WithName(worker.Name), claudemd.WithDescription(worker.Description), claudemd.WithMemory(worker.Memory)); err != nil {
		log.Error("ensure system rules", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.beeCfg.Claude.Timeout

	if err := m.launchRuntime(exec, worker, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, starts the invoker, registers the process, updates PID, and launches monitoring.
// The execution context is always derived from context.Background() to decouple from the caller's request.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, timeout time.Duration, prompt string, resume bool) error {
	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		execCtx, cancel = context.WithCancel(context.Background())
	}

	proc, outputCh, err := m.invoker.Run(execCtx, worker.WorkDir, prompt, claude.RunOptions{SessionID: exec.SessionID, Resume: resume})
	if err != nil {
		cancel()
		return err
	}

	el := &executionLog{}
	m.mu.Lock()
	m.activeProcesses[exec.ID] = proc
	m.activeLogs[exec.ID] = el
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, proc.PID())
	go m.monitorExecution(exec, worker, outputCh, cancel)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, outputCh <-chan claude.Output, cancel context.CancelFunc) {
	defer cancel()
	m.mu.RLock()
	activeLog := m.activeLogs[exec.ID]
	m.mu.RUnlock()

	var lastAssistantText string
	var streamResult string

	for out := range outputCh {
		switch out.Type {
		case claude.OutputStdout:
			activeLog.writeLine(out.Content)
			// Parse stream-json to extract assistant text and result
			line := strings.TrimSpace(out.Content)
			if strings.HasPrefix(line, "{") {
				var event claudeStreamEvent
				if err := json.Unmarshal([]byte(line), &event); err == nil {
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
				}
			}
		case claude.OutputDone:
			rawLogs := activeLog.string()
			// Save raw stdout logs
			if _, err := m.executionStore.WriteLog(exec.ID, exec.StartedAt, rawLogs); err != nil {
				log.Error("write execution logs", zap.Error(err))
			}
			// Determine result with priority: streamResult > lastAssistantText > rawLogs
			result := rawLogs
			if lastAssistantText != "" {
				result = lastAssistantText
			}
			if streamResult != "" {
				result = streamResult
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case claude.OutputError:
			rawLogs := activeLog.string()
			if _, err := m.executionStore.WriteLog(exec.ID, exec.StartedAt, rawLogs); err != nil {
				log.Error("write execution logs", zap.Error(err))
			}
			m.executionStore.UpdateResult(exec.ID, rawLogs+"\nERROR: "+out.Content, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	// Cleanup
	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	delete(m.activeLogs, exec.ID)
	m.mu.Unlock()
}

func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error {
	if deleteWorkDir {
		worker, err := m.workerStore.GetByID(id)
		if err != nil {
			return fmt.Errorf("get worker: %w", err)
		}
		if worker.WorkDir != "" {
			if err := os.RemoveAll(worker.WorkDir); err != nil {
				return fmt.Errorf("remove work dir: %w", err)
			}
		}
	}
	return m.workerStore.Delete(id)
}

func (m *Manager) StopExecution(executionID string) error {
	m.mu.RLock()
	proc, ok := m.activeProcesses[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active process for execution %s", executionID)
	}
	return proc.Stop()
}

