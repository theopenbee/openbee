package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeTimedTaskLister struct {
	tasks   []model.Task
	err     error
	filters []store.TaskFilter
}

func (f *fakeTimedTaskLister) List(_ context.Context, fl store.TaskFilter) ([]model.Task, error) {
	f.filters = append(f.filters, fl)
	return f.tasks, f.err
}

type fakeTaskCanceller struct {
	cancelled []string
	err       error
}

func (f *fakeTaskCanceller) CancelTask(_ context.Context, id string) error {
	f.cancelled = append(f.cancelled, id)
	return f.err
}

var (
	countdownAt = time.Date(2026, 11, 12, 20, 30, 0, 0, time.Local).UnixMilli()
	nextDaily   = time.Date(2026, 11, 13, 9, 0, 0, 0, time.Local).UnixMilli()
	timedTasks  = []model.Task{
		{ID: "aaaa1111-0000-0000-0000-000000000001", WorkerID: "w1", Instruction: "提醒喝水", Type: model.TaskTypeCountdown, Status: model.TaskStatusPending, ScheduledAt: &countdownAt},
		{ID: "bbbb2222-0000-0000-0000-000000000002", WorkerID: "w1", Instruction: "发日报", Type: model.TaskTypeScheduled, Status: model.TaskStatusPending, CronExpr: "0 9 * * *", NextRunAt: &nextDaily},
	}
)

func makeCancelHandler(tasks []model.Task) (*command.CancelCommandHandler, *fakeSender, *fakeTimedTaskLister, *fakeTaskCanceller) {
	sender := &fakeSender{}
	lister := &fakeTimedTaskLister{tasks: tasks}
	canceller := &fakeTaskCanceller{}
	workers := &fakeWorkerByIDsLookup{byID: map[string]model.Worker{"w1": {ID: "w1", Name: "tester"}}}
	h := command.NewCancelCommandHandler(lister, canceller, workers, platform.SenderMap{"beta": sender})
	return h, sender, lister, canceller
}

func TestCancelCommand_IsCommand(t *testing.T) {
	h, _, _, _ := makeCancelHandler(nil)
	assert.True(t, h.IsCommand("/cancel"))
	assert.True(t, h.IsCommand("/cancel abcd1234"))
	assert.False(t, h.IsCommand("/cancelled"))
}

func TestCancelCommand_ListsTimedTasksWithButtons(t *testing.T) {
	h, sender, lister, _ := makeCancelHandler(timedTasks)
	require.True(t, h.HandleCommand(context.Background(), "/cancel", makeReplyTo()))

	assert.Equal(t, []store.TaskFilter{{
		SessionKey: "beta:chat1:user1",
		Status:     "pending,running",
		Type:       "countdown,scheduled",
	}}, lister.filters)
	assert.Equal(t, []string{"当前会话的定时和倒计时任务（2）：\n" +
		"  1. [tester] 提醒喝水 · 2026-11-12 20:30   ID: aaaa1111\n" +
		"  2. [tester] 发日报 · 0 9 * * *（下次 2026-11-13 09:00）   ID: bbbb2222\n" +
		"\n发送 /cancel {任务ID} 取消。"}, sender.sent)
	assert.Equal(t, []platform.Button{
		{Label: "取消 1", Text: "/cancel aaaa1111", Style: platform.ButtonStyleDanger},
		{Label: "取消 2", Text: "/cancel bbbb2222", Style: platform.ButtonStyleDanger},
	}, sender.buttons[0])
}

func TestCancelCommand_NothingToList(t *testing.T) {
	h, sender, _, _ := makeCancelHandler(nil)
	h.HandleCommand(context.Background(), "/cancel", makeReplyTo())
	assert.Equal(t, []string{"当前会话没有定时或倒计时任务。"}, sender.sent)
	assert.Empty(t, sender.buttons[0])
}

func TestCancelCommand_CancelsByIDPrefix(t *testing.T) {
	h, sender, _, canceller := makeCancelHandler(timedTasks)
	h.HandleCommand(context.Background(), "/cancel bbbb2222", makeReplyTo())
	assert.Equal(t, []string{"bbbb2222-0000-0000-0000-000000000002"}, canceller.cancelled)
	assert.Equal(t, []string{"✅ 已取消：[tester] 发日报"}, sender.sent)
}

func TestCancelCommand_UnknownOrTooShortID(t *testing.T) {
	for _, id := range []string{"cccc3333", "aaa"} {
		h, sender, _, canceller := makeCancelHandler(timedTasks)
		h.HandleCommand(context.Background(), "/cancel "+id, makeReplyTo())
		assert.Empty(t, canceller.cancelled)
		assert.Equal(t, []string{"没有找到任务 " + id + "，它可能已经执行完或被取消了。"}, sender.sent)
	}
}

func TestCancelCommand_AmbiguousPrefixListsMatches(t *testing.T) {
	tasks := []model.Task{
		{ID: "abcd0001", WorkerID: "w1", Instruction: "a", Type: model.TaskTypeCountdown, ScheduledAt: &countdownAt},
		{ID: "abcd0002", WorkerID: "w1", Instruction: "b", Type: model.TaskTypeCountdown, ScheduledAt: &countdownAt},
		{ID: "ffff0003", WorkerID: "w1", Instruction: "c", Type: model.TaskTypeCountdown, ScheduledAt: &countdownAt},
	}
	h, sender, _, canceller := makeCancelHandler(tasks)
	h.HandleCommand(context.Background(), "/cancel abcd", makeReplyTo())
	assert.Empty(t, canceller.cancelled)
	assert.Contains(t, sender.sent[0], "（2）")
	assert.Len(t, sender.buttons[0], 2)
}

func TestCancelCommand_Failures(t *testing.T) {
	h, sender, lister, canceller := makeCancelHandler(timedTasks)
	canceller.err = errors.New("boom")
	h.HandleCommand(context.Background(), "/cancel aaaa1111", makeReplyTo())
	assert.Equal(t, "⚠️ 取消失败，请稍后重试。", sender.sent[0])

	lister.err = errors.New("db down")
	h.HandleCommand(context.Background(), "/cancel", makeReplyTo())
	assert.Equal(t, "⚠️ 查询定时任务失败，请稍后重试；若持续出现请检查服务端日志。", sender.sent[1])

	h.HandleCommand(context.Background(), "/cancel a b", makeReplyTo())
	assert.Contains(t, sender.sent[2], "用法：")
}

func TestTimedTaskNotice(t *testing.T) {
	content, buttons := command.TimedTaskNotice(timedTasks[0], "tester")
	assert.Equal(t, "⏰ 已创建倒计时任务\n员工：tester\n内容：提醒喝水\n执行时间：2026-11-12 20:30\n\n发送 /cancel aaaa1111 可取消。", content)
	assert.Equal(t, []platform.Button{{Label: "取消任务", Text: "/cancel aaaa1111", Style: platform.ButtonStyleDanger}}, buttons)

	content, _ = command.TimedTaskNotice(timedTasks[1], "tester")
	assert.Equal(t, "⏰ 已创建定时任务\n员工：tester\n内容：发日报\n周期：0 9 * * *\n下次执行：2026-11-13 09:00\n\n发送 /cancel bbbb2222 可取消。", content)
}
