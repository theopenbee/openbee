package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
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

	engineName, err := m.resolveEngine(worker)
	if err != nil {
		return model.WorkerExecution{}, err
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := m.bridge.PrepareWorkerWorkspace(worker.WorkDir, engineName); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.workerTimeout

	if err := m.launchRuntime(ctx, exec, worker, engineName, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

func (m *Manager) launchRuntime(ctx context.Context, exec model.WorkerExecution, worker model.Worker, engineName string, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		execCtx, cancel = context.WithCancel(ctx)
	}

	runRes, err := m.bridge.RunWorker(execCtx, bridge.WorkerRunRequest{
		WorkerID:         worker.ID,
		WorkDir:          worker.WorkDir,
		PermissionScopes: utils.SplitAndTrim(worker.PermissionScopes),
		WorkerEngine:     engineName,
		WorkerEngineArgs: worker.EngineArgs,
		Prompt:           prompt,
		SessionID:        exec.SessionID,
		Resume:           resume,
		LogPath:          logPath,
	})
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = runRes.Process
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, runRes.Process.PID())
	go m.monitorExecution(exec, worker, runRes, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, runRes bridge.RunHandle, cancel context.CancelFunc, logPath string) {
	defer cancel()

	finalized := false
	for event := range runRes.Events {
		switch event.Type {
		case bridge.LifecycleDone:
			result := runRes.ExtractResult(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
			finalized = true
		case bridge.LifecycleError:
			result := runRes.ExtractResult(logPath)
			if result == "" {
				result = event.Content
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
			finalized = true
		}
	}

	if !finalized {
		// Output channel closed without a terminal Done/Error signal —
		// process was killed, crashed, or signal-terminated. Without this
		// fallback the execution would stay in `running` forever.
		result := runRes.ExtractResult(logPath)
		if result == "" {
			result = "process exited without completion signal"
		}
		if _, err := m.executionStore.MarkAbandoned(context.Background(), exec.ID, result); err != nil {
			log.Error("finalize abandoned execution", zap.String("executionID", exec.ID), zap.Error(err))
		}
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	m.mu.Unlock()
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
