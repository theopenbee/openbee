package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var log = logger.With(zap.String("component", "worker"))

type Manager struct {
	workerBaseDir  string
	tokenSecret    string
	tokenTTL       time.Duration
	engineTimeouts map[string]time.Duration // per-engine worker execution timeout; 0 = no timeout
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	engines        map[string]ai.EngineAdapter
	defaultEngine  string
	envService     *env.Service

	activeProcesses map[string]ai.Process // execution_id -> process
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	engines map[string]ai.EngineAdapter,
	envService *env.Service,
) *Manager {
	timeouts := make(map[string]time.Duration, len(ai.AllEngines))
	for _, name := range ai.AllEngines {
		timeouts[name] = bc.WorkerTimeoutFor(name)
	}
	return &Manager{
		workerBaseDir:   workerBaseDir,
		tokenSecret:     bc.MCP.TokenSecret,
		tokenTTL:        bc.MCP.TokenTTL,
		engineTimeouts:  timeouts,
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		defaultEngine:   bc.EffectiveEngine(),
		envService:      envService,
		activeProcesses: make(map[string]ai.Process),
	}
}

// resolveTimeout returns the execution timeout for the given engine name, falling back to the default engine.
func (m *Manager) resolveTimeout(engineName string) time.Duration {
	if t, ok := m.engineTimeouts[engineName]; ok {
		return t
	}
	return m.engineTimeouts[m.defaultEngine]
}

// resolveEngine returns the EngineAdapter for w, falling back to the default if w.Engine is empty or unknown.
func (m *Manager) resolveEngine(w model.Worker) (ai.EngineAdapter, error) {
	if w.Engine != "" {
		if e, ok := m.engines[w.Engine]; ok {
			return e, nil
		}
		log.Warn("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	if e, ok := m.engines[m.defaultEngine]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("no engine adapter found (worker engine %q, default %q)", w.Engine, m.defaultEngine)
}

// EnabledEngines returns the names of all currently enabled engines.
func (m *Manager) EnabledEngines() []string {
	names := make([]string, 0, len(m.engines))
	for name := range m.engines {
		names = append(names, name)
	}
	return names
}

// CreateWorkerParams holds the inputs for creating a new worker.
type CreateWorkerParams struct {
	Name             string
	Description      string
	Memory           string
	WorkDir          string
	PermissionScopes string
	Engine           string
}

func (m *Manager) CreateWorker(p CreateWorkerParams) (model.Worker, error) {
	id := uuid.New().String()
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(p.WorkDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	workerModel := model.Worker{
		ID:               id,
		Name:             p.Name,
		Description:      p.Description,
		Memory:           p.Memory,
		WorkDir:          p.WorkDir,
		Engine:           p.Engine,
		PermissionScopes: p.PermissionScopes,
	}
	engine, err := m.resolveEngine(workerModel)
	if err != nil {
		return model.Worker{}, err
	}
	if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
	}

	return m.workerStore.Create(workerModel)
}

// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	engine, err := m.resolveEngine(worker)
	if err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, err
	}
	if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.resolveTimeout(worker.Engine)

	if err := m.launchRuntime(exec, worker, engine, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, prepares the log path, starts the invoker,
// registers the process, updates PID, and launches monitoring.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	token, err := auth.GenerateWorkerToken(m.tokenSecret, worker.ID, utils.SplitAndTrim(worker.PermissionScopes), m.tokenTTL)
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

	extraEnv, err := m.envService.ResolveWorkerEnv(worker.ID)
	if err != nil {
		cancel()
		return fmt.Errorf("resolve worker env: %w", err)
	}

	proc, outputCh, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID: exec.SessionID,
		Resume:    resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
	}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = proc
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, proc.PID())
	go m.monitorExecution(exec, worker, engine, outputCh, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, outputCh <-chan ai.Output, cancel context.CancelFunc, logPath string) {
	defer cancel()

	for out := range outputCh {
		switch out.Type {
		case ai.OutputDone:
			result := engine.ExtractResult(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case ai.OutputError:
			result := engine.ExtractResult(logPath)
			if result == "" {
				result = out.Content
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
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

// CancelExecution implements task.ExecutionManager.
// It stops the active worker process for the given executionID.
func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
