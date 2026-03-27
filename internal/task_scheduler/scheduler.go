package task_scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
)

var log = logger.With(zap.String("component", "taskscheduler"))

// Scheduler polls for due tasks and sends them to the TaskDispatcher.
type Scheduler struct {
	taskStore    *store.TaskStore
	dispatchCh   chan<- task_dispatcher.DispatchTask
	pollInterval time.Duration
}

// New creates a Scheduler.
func New(taskStore *store.TaskStore, dispatchCh chan<- task_dispatcher.DispatchTask, pollInterval time.Duration) *Scheduler {
	return &Scheduler{
		taskStore:    taskStore,
		dispatchCh:   dispatchCh,
		pollInterval: pollInterval,
	}
}

// RecoverRunning resets all 'running' tasks to 'pending'.
// Must be called synchronously at startup AFTER the Feeder's RecoverFeeding.
func (s *Scheduler) RecoverRunning(ctx context.Context) {
	n, err := s.taskStore.ResetRunningToPending(ctx)
	if err != nil {
		log.Error("recover running tasks", zap.Error(err))
		return
	}
	if n > 0 {
		log.Info("reset running tasks to pending", zap.Int64("count", n))
	}
}

// Run polls for due tasks on each tick until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.poll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) poll(ctx context.Context) {
	nowMS := time.Now().UnixMilli()
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS, nil) // will be fixed in Task 3
	if err != nil {
		log.Error("claim due tasks", zap.Error(err))
		return
	}

	for _, ct := range tasks {
		sessionKey := ct.MessageSessionKey
		dt := task_dispatcher.DispatchTask{
			TaskID:      ct.ID,
			WorkerID:    ct.WorkerID,
			SessionKey:  sessionKey,
			Instruction: ct.Instruction,
			ReplyTo:     platform.InboundMessage{Platform: ct.MessagePlatform, SessionKey: sessionKey},
			TaskType:    ct.Type,
			MessageID:   ct.MessageID,
		}
		select {
		case s.dispatchCh <- dt:
		case <-ctx.Done():
			return
		}
	}
}
