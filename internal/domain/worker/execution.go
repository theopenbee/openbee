package worker

import (
	"context"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"go.uber.org/zap"
)

// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	engineName, engine := m.resolveEngine(worker)

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.workerTimeout

	if err := m.launchRuntime(ctx, exec, worker, engine, engineName, timeout, triggerInput, resume, m.workerStore, "worker"); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// ExecuteAgent runs an agent backed by worker-compatible execution settings.
// It is used for non-worker agents, such as Groups, that have their own
// persisted metadata but still run through the worker engine runtime.
func (m *Manager) ExecuteAgent(ctx context.Context, agent model.Worker, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	if agent.ID == "" {
		return model.WorkerExecution{}, fmt.Errorf("agent id is required")
	}
	engineName, engine, err := m.resolveEngineSelection(agent)
	if err != nil {
		m.updateStatus(m.groupStatus, agent.ID, model.WorkerStatusError, "group")
		return model.WorkerExecution{}, err
	}

	exec, err := m.executionStore.Create(agent.ID, triggerInput, sessionID, engineName)
	if err != nil {
		m.updateStatus(m.groupStatus, agent.ID, model.WorkerStatusError, "group")
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	m.updateStatus(m.groupStatus, agent.ID, model.WorkerStatusWorking, "group")

	if err := engine.Prepare(agent.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare agent workspace", zap.String("op", "execute_agent"), zap.String("agentID", agent.ID), zap.Error(err))
	}

	if err := m.launchRuntime(ctx, exec, agent, engine, engineName, m.workerTimeout, triggerInput, resume, m.groupStatus, "group"); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.updateStatus(m.groupStatus, agent.ID, model.WorkerStatusError, "group")
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

func (m *Manager) launchRuntime(ctx context.Context, exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, engineName string, timeout time.Duration, prompt string, resume bool, statusStore statusUpdater, statusLabel string) error {
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

	extraArgs := m.resolveEngineArgs(ctx, worker, engineName)

	runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID: exec.SessionID,
		Resume:    resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
		ExtraArgs: extraArgs,
	}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = runRes.Process
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, runRes.Process.PID())
	go m.monitorExecution(exec, worker, runRes, cancel, logPath, statusStore, statusLabel)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, runRes ai.RunResult, cancel context.CancelFunc, logPath string, statusStore statusUpdater, statusLabel string) {
	defer cancel()

	for out := range runRes.Output {
		switch out.Type {
		case ai.OutputDone:
			result := runRes.ExtractResult(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.updateStatus(statusStore, worker.ID, model.WorkerStatusIdle, statusLabel)
		case ai.OutputError:
			result := runRes.ExtractResult(logPath)
			if result == "" {
				result = out.Content
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
			m.updateStatus(statusStore, worker.ID, model.WorkerStatusError, statusLabel)
		}
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	m.mu.Unlock()
}

func (m *Manager) updateStatus(statusStore statusUpdater, id string, status model.WorkerStatus, label string) {
	if statusStore == nil {
		return
	}
	if err := statusStore.UpdateStatus(id, status); err != nil {
		log.Error("failed to update agent status",
			zap.String("agent_id", id),
			zap.String("agent_type", label),
			zap.String("status", string(status)),
			zap.Error(err))
	}
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
