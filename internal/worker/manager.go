package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robobee/core/internal/claudemd"
	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/model"
	"github.com/robobee/core/internal/store"
)

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

type Manager struct {
	workerBaseDir  string
	beeCfg         config.BeeConfig
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore

	activeRuntimes map[string]Runtime      // execution_id -> runtime
	logSubscribers map[string][]chan Output // execution_id -> subscribers
	mu             sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
) *Manager {
	return &Manager{
		workerBaseDir:  workerBaseDir,
		beeCfg:         bc,
		workerStore:    ws,
		executionStore: es,
		activeRuntimes: make(map[string]Runtime),
		logSubscribers: make(map[string][]chan Output),
	}
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

	if err := claudemd.EnsureSystemRules(workDir, claudemd.RoleWorker, claudemd.WithName(name), claudemd.WithDescription(description)); err != nil {
		slog.Error("ensure system rules", "component", "worker", "op", "create", "error", err)
	}

	return m.workerStore.Create(model.Worker{
		ID:          id,
		Name:        name,
		Description: description,
		Memory:      memory,
		WorkDir:     workDir,
	})
}

func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput string) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	exec, err := m.executionStore.Create(workerID, triggerInput)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	// Update worker status
	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		slog.Error("failed to update worker status", "component", "worker", "error", err)
	}

	if err := claudemd.EnsureSystemRules(worker.WorkDir, claudemd.RoleWorker, claudemd.WithName(worker.Name), claudemd.WithDescription(worker.Description)); err != nil {
		slog.Error("ensure system rules", "component", "worker", "op", "execute", "error", err)
	}

	rt := NewClaudeRuntime(m.beeCfg.Claude.Path, m.beeCfg.MCPBaseURL, m.beeCfg.MCP.APIKey)
	timeout := m.beeCfg.Claude.Timeout

	// Build the prompt: memory + trigger input
	prompt := worker.Memory
	if triggerInput != "" {
		if worker.Memory != "" {
			prompt = fmt.Sprintf("%s\n\n---\nMessage:\n%s", worker.Memory, triggerInput)
		} else {
			prompt = triggerInput
		}
	}

	if err := m.launchRuntime(exec, worker, rt, timeout, prompt, false); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// ExecuteWorkerWithSession runs a worker resuming an existing Claude session identified by sessionID.
// This is used by the TaskDispatcher when a prior session exists for the (sessionKey, workerID) pair.
func (m *Manager) ExecuteWorkerWithSession(ctx context.Context, workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	exec, err := m.executionStore.CreateWithSessionID(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution with session: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		slog.Error("failed to update worker status", "component", "worker", "error", err)
	}

	if err := claudemd.EnsureSystemRules(worker.WorkDir, claudemd.RoleWorker, claudemd.WithName(worker.Name), claudemd.WithDescription(worker.Description)); err != nil {
		slog.Error("ensure system rules", "component", "worker", "op", "executeWithSession", "error", err)
	}

	rt := NewClaudeRuntime(m.beeCfg.Claude.Path, m.beeCfg.MCPBaseURL, m.beeCfg.MCP.APIKey)
	timeout := m.beeCfg.Claude.Timeout

	// On resume, only the new message is sent — the worker's base prompt is already
	// established in the Claude session history (same as ReplyExecution).
	if err := m.launchRuntime(exec, worker, rt, timeout, triggerInput, true); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, starts the runtime, registers it, updates PID, and launches monitoring.
// The execution context is always derived from context.Background() to decouple from the caller's request.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, rt Runtime, timeout time.Duration, prompt string, resume bool) error {
	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		execCtx, cancel = context.WithCancel(context.Background())
	}

	outputCh, err := rt.Execute(execCtx, worker.WorkDir, prompt, ExecuteOptions{SessionID: exec.SessionID, Resume: resume})
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeRuntimes[exec.ID] = rt
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, rt.PID())
	go m.monitorExecution(exec, worker, outputCh, cancel)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, outputCh <-chan Output, cancel context.CancelFunc) {
	defer cancel()
	var rawLogsBuilder strings.Builder
	var lastAssistantText string
	var streamResult string

	for out := range outputCh {
		// Broadcast to WebSocket subscribers
		m.mu.RLock()
		subs := m.logSubscribers[exec.ID]
		m.mu.RUnlock()

		for _, sub := range subs {
			select {
			case sub <- out:
			default:
			}
		}

		switch out.Type {
		case OutputStdout:
			rawLogsBuilder.WriteString(out.Content)
			rawLogsBuilder.WriteByte('\n')
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
		case OutputDone:
			rawLogs := rawLogsBuilder.String()
			// Save raw stdout logs
			m.executionStore.UpdateLogs(exec.ID, rawLogs)
			// Determine result with priority: file > streamResult > lastAssistantText > rawLogs
			result := rawLogs
			if lastAssistantText != "" {
				result = lastAssistantText
			}
			if streamResult != "" {
				result = streamResult
			}
			resultFilePath := filepath.Join(worker.WorkDir, ".robobee_result.txt")
			if data, err := os.ReadFile(resultFilePath); err == nil && len(data) > 0 {
				result = string(data)
				os.Remove(resultFilePath)
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case OutputError:
			rawLogs := rawLogsBuilder.String()
			m.executionStore.UpdateLogs(exec.ID, rawLogs)
			m.executionStore.UpdateResult(exec.ID, rawLogs+"\nERROR: "+out.Content, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	// Cleanup
	m.mu.Lock()
	delete(m.activeRuntimes, exec.ID)
	for _, sub := range m.logSubscribers[exec.ID] {
		close(sub)
	}
	delete(m.logSubscribers, exec.ID)
	m.mu.Unlock()
}

func (m *Manager) SubscribeLogs(executionID string) <-chan Output {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan Output, 100)
	m.logSubscribers[executionID] = append(m.logSubscribers[executionID], ch)
	return ch
}

func (m *Manager) ReplyExecution(ctx context.Context, executionID string, message string) (model.WorkerExecution, error) {
	srcExec, err := m.executionStore.GetByID(executionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get execution: %w", err)
	}
	if srcExec.Status == model.ExecStatusRunning || srcExec.Status == model.ExecStatusPending {
		return model.WorkerExecution{}, fmt.Errorf("execution is still running")
	}

	worker, err := m.workerStore.GetByID(srcExec.WorkerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}
	newExec, err := m.executionStore.CreateWithSessionID(srcExec.WorkerID, message, srcExec.SessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create reply execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		slog.Error("failed to update worker status", "component", "worker", "error", err)
	}

	if err := claudemd.EnsureSystemRules(worker.WorkDir, claudemd.RoleWorker, claudemd.WithName(worker.Name), claudemd.WithDescription(worker.Description)); err != nil {
		slog.Error("ensure system rules", "component", "worker", "op", "reply", "error", err)
	}

	rt := NewClaudeRuntime(m.beeCfg.Claude.Path, m.beeCfg.MCPBaseURL, m.beeCfg.MCP.APIKey)
	timeout := m.beeCfg.Claude.Timeout

	if err := m.launchRuntime(newExec, worker, rt, timeout, message, true); err != nil {
		m.executionStore.UpdateResult(newExec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return newExec, fmt.Errorf("start runtime: %w", err)
	}

	return newExec, nil
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

// GetExecution returns the current state of an execution by ID.
func (m *Manager) GetExecution(id string) (model.WorkerExecution, error) {
	return m.executionStore.GetByID(id)
}

func (m *Manager) StopExecution(executionID string) error {
	m.mu.RLock()
	rt, ok := m.activeRuntimes[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active runtime for execution %s", executionID)
	}
	return rt.Stop()
}
