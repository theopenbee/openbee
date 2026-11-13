package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

const (
	CmdCancel       = "/cancel"
	maxListedTimed  = 10
	shortTaskIDLen  = 8
	minTaskIDPrefix = 4
	timedTimeLayout = "2006-01-02 15:04"
)

type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

type CancelCommandHandler struct {
	tasks     TaskBySessionLister
	canceller TaskCanceller
	workers   WorkerByIDsLookup
	senders   platform.Senders
}

func NewCancelCommandHandler(tasks TaskBySessionLister, canceller TaskCanceller, workers WorkerByIDsLookup, senders platform.Senders) *CancelCommandHandler {
	return &CancelCommandHandler{tasks: tasks, canceller: canceller, workers: workers, senders: senders}
}

func (h *CancelCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdCancel)
}

func (h *CancelCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdCancel {
		return false
	}
	m := i18n.M.Runtime.CancelCommand
	if len(fields) > 2 {
		sendReply(ctx, h.senders, replyTo, m.Usage)
		return true
	}

	tasks, err := h.tasks.List(ctx, store.TaskFilter{
		SessionKey: replyTo.SessionKey,
		Status:     model.TaskStatusPending + "," + model.TaskStatusRunning,
		Type:       model.TaskTypeCountdown + "," + model.TaskTypeScheduled,
	})
	if err != nil {
		log.Error("list timed tasks for /cancel", zap.String("sessionKey", replyTo.SessionKey), zap.Error(err))
		sendReply(ctx, h.senders, replyTo, m.LookupFailed)
		return true
	}
	names := resolveWorkerNames(h.workers, tasks)

	if len(fields) == 1 {
		h.replyList(ctx, replyTo, tasks, names)
		return true
	}
	matches := matchTaskID(tasks, fields[1])
	switch len(matches) {
	case 0:
		sendReply(ctx, h.senders, replyTo, fmt.Sprintf(m.NotFound, fields[1]))
	case 1:
		t := matches[0]
		if err := h.canceller.CancelTask(ctx, t.ID); err != nil {
			log.Error("cancel timed task", zap.String("taskID", t.ID), zap.Error(err))
			sendReply(ctx, h.senders, replyTo, m.CancelFailed)
			return true
		}
		sendReply(ctx, h.senders, replyTo, fmt.Sprintf(m.Cancelled, workerNameOrFallback(names, t.WorkerID), instructionSummary(t)))
	default:
		h.replyList(ctx, replyTo, matches, names)
	}
	return true
}

func (h *CancelCommandHandler) replyList(ctx context.Context, replyTo platform.InboundMessage, tasks []model.Task, names map[string]string) {
	m := i18n.M.Runtime.CancelCommand
	if len(tasks) == 0 {
		sendReply(ctx, h.senders, replyTo, m.Empty)
		return
	}
	lines := []string{fmt.Sprintf(m.Header, len(tasks))}
	shown := tasks[:min(len(tasks), maxListedTimed)]
	buttons := make([]platform.Button, 0, len(shown))
	for i, t := range shown {
		id := shortTaskID(t.ID)
		lines = append(lines, fmt.Sprintf(m.Line, i+1, workerNameOrFallback(names, t.WorkerID), instructionSummary(t), timedTaskWhen(t), id))
		buttons = append(buttons, platform.Button{
			Label: fmt.Sprintf(m.Button, i+1),
			Text:  CmdCancel + " " + id,
			Style: platform.ButtonStyleDanger,
		})
	}
	lines = append(lines, "", m.Footer)
	sendReplyWithButtons(ctx, h.senders, replyTo, strings.Join(lines, "\n"), buttons)
}

func TimedTaskNotice(t model.Task, workerName string) (string, []platform.Button) {
	m := i18n.M.Runtime.CancelCommand
	id := shortTaskID(t.ID)
	var body string
	if t.Type == model.TaskTypeScheduled {
		body = fmt.Sprintf(m.ScheduledCreated, workerName, instructionSummary(t), t.CronExpr, formatTimedMillis(t.NextRunAt))
	} else {
		body = fmt.Sprintf(m.CountdownCreated, workerName, instructionSummary(t), formatTimedMillis(t.ScheduledAt))
	}
	content := body + "\n\n" + fmt.Sprintf(m.CreatedFooter, id)
	return content, []platform.Button{{Label: m.CancelButton, Text: CmdCancel + " " + id, Style: platform.ButtonStyleDanger}}
}

func matchTaskID(tasks []model.Task, prefix string) []model.Task {
	if len(prefix) < minTaskIDPrefix {
		return nil
	}
	var out []model.Task
	for _, t := range tasks {
		if strings.HasPrefix(t.ID, prefix) {
			out = append(out, t)
		}
	}
	return out
}

func timedTaskWhen(t model.Task) string {
	if t.Type == model.TaskTypeScheduled {
		return fmt.Sprintf(i18n.M.Runtime.CancelCommand.ScheduledWhen, t.CronExpr, formatTimedMillis(t.NextRunAt))
	}
	return formatTimedMillis(t.ScheduledAt)
}

func formatTimedMillis(ms *int64) string {
	if ms == nil {
		return "-"
	}
	return time.UnixMilli(*ms).Local().Format(timedTimeLayout)
}

func instructionSummary(t model.Task) string {
	return utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes)
}

func shortTaskID(id string) string {
	return id[:min(len(id), shortTaskIDLen)]
}
