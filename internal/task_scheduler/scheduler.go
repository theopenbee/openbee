package task_scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
)

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
		slog.Error("recover running tasks", "component", "taskscheduler", "error", err)
		return
	}
	if n > 0 {
		slog.Info("reset running tasks to pending", "component", "taskscheduler", "count", n)
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
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS)
	if err != nil {
		slog.Error("claim due tasks", "component", "taskscheduler", "error", err)
		return
	}

	for _, ct := range tasks {
		// For scheduled tasks, compute the real next_run_at and update.
		if ct.Type == model.TaskTypeScheduled && ct.CronExpr != "" {
			sched, err := cron.ParseStandard(ct.CronExpr)
			if err != nil {
				slog.Error("invalid cron expression", "component", "taskscheduler", "cronExpr", ct.CronExpr, "taskID", ct.ID, "error", err)
				s.taskStore.SetExecution(ctx, ct.ID, "", model.TaskStatusFailed) //nolint:errcheck
				continue
			}
			next := sched.Next(time.Now()).UnixMilli()
			s.taskStore.UpdateNextRunAt(ctx, ct.ID, next) //nolint:errcheck
		}

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
