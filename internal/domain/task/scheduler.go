package task

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

var schedulerLog = logger.With(zap.String("component", "taskscheduler"))

type schedulerStore interface {
	PeekDueScheduledTasks(ctx context.Context, nowMS int64) ([]model.Task, error)
	ClaimDueTasks(ctx context.Context, nowMS int64, scheduledNextRuns map[string]int64) ([]model.ClaimedTask, error)
	ResetRunningToPending(ctx context.Context) (int64, error)
	UpdateStatus(ctx context.Context, taskID, status string) error
}

// Scheduler polls for due tasks and sends them to the TaskDispatcher.
type Scheduler struct {
	taskStore    schedulerStore
	dispatchCh   chan<- DispatchTask
	pollInterval time.Duration
}

// NewScheduler creates a Scheduler.
func NewScheduler(taskStore schedulerStore, dispatchCh chan<- DispatchTask, pollInterval time.Duration) *Scheduler {
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

	// Step 1: read-only peek at due scheduled tasks to compute real next_run_at values.
	peeked, err := s.taskStore.PeekDueScheduledTasks(ctx, nowMS)
	if err != nil {
		log.Error("peek due scheduled tasks", zap.Error(err))
		return
	}
	scheduledNextRuns := make(map[string]int64, len(peeked))
	for _, t := range peeked {
		if t.CronExpr == "" {
			continue
		}
		sched, err := cron.ParseStandard(t.CronExpr)
		if err != nil {
			log.Error("invalid cron expression", zap.String("cronExpr", t.CronExpr), zap.String("taskID", t.ID), zap.Error(err))
			continue
		}
		scheduledNextRuns[t.ID] = sched.Next(time.Now()).UnixMilli()
	}

	// Step 2: claim all due tasks atomically, writing real next_run_at in the same transaction.
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS, scheduledNextRuns)
	if err != nil {
		log.Error("claim due tasks", zap.Error(err))
		return
	}

	for _, ct := range tasks {
		// Scheduled tasks with invalid cron were skipped in peek; skip dispatch too.
		if ct.Type == model.TaskTypeScheduled && ct.CronExpr != "" {
			if _, ok := scheduledNextRuns[ct.ID]; !ok {
				s.taskStore.UpdateStatus(ctx, ct.ID, model.TaskStatusFailed) //nolint:errcheck
				continue
			}
		}

		sessionKey := ct.MessageSessionKey
		dt := DispatchTask{
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
