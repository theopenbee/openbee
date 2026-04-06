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
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
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

type Manager struct {
	workerBaseDir  string
	beeCfg         config.BeeConfig
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	engine         ai.EngineAdapter

	activeProcesses map[string]ai.Process // execution_id -> process
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	engine ai.EngineAdapter,
) *Manager {
	return &Manager{
		workerBaseDir:   workerBaseDir,
		beeCfg:          bc,
		workerStore:     ws,
		executionStore:  es,
		engine:          engine,
		activeProcesses: make(map[string]ai.Process),
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

	if err := m.engine.SetupWorkspace(workDir, ai.RoleWorker, ai.WorkspaceOptions{
		Name:        name,
		Description: description,
		Memory:      memory,
	}); err != nil {
		log.Error("setup worker workspace", zap.String("op", "create"), zap.Error(err))
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

	if err := m.engine.SetupWorkspace(worker.WorkDir, ai.RoleWorker, ai.WorkspaceOptions{
		Name:        worker.Name,
		Description: worker.Description,
		Memory:      worker.Memory,
	}); err != nil {
		log.Error("setup worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.beeCfg.Claude.Timeout

	if err := m.launchRuntime(exec, worker, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, prepares the log path, starts the invoker,
// registers the process, updates PID, and launches monitoring.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	token, err := auth.GenerateWorkerToken(m.beeCfg.MCP.TokenSecret, worker.ID, m.beeCfg.MCP.TokenTTL)
	if err != nil {
		return fmt.Errorf("generate worker token: %w", err)
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		execCtx, cancel = context.WithCancel(context.Background())
	}

	proc, outputCh, err := m.engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID: exec.SessionID,
		Resume:    resume,
		APIKey:    token,
	}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = proc
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, proc.PID())
	go m.monitorExecution(exec, worker, outputCh, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, outputCh <-chan ai.Output, cancel context.CancelFunc, logPath string) {
	defer cancel()

	for out := range outputCh {
		switch out.Type {
		case ai.OutputDone:
			result := extractResultFromLog(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case ai.OutputError:
			m.executionStore.UpdateResult(exec.ID, out.Content, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	m.mu.Unlock()
}

// extractResultFromLog scans the log file for stream-json events and returns
// the best result string: prefers {"type":"result"} over the last assistant text.
func extractResultFromLog(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	var lastAssistantText, streamResult string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event claudeStreamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
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
	if streamResult != "" {
		return streamResult
	}
	return lastAssistantText
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

// CancelExecution implements task.ExecutionManager.
// It stops the active worker process for the given executionID.
func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
