package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes for /clear ---

type fakeClearSessionStore struct {
	agents          []store.SessionAgent
	listErr         error
	deleted         bool
	deleteErr       error
	deletedSessions []string // captured calls for assertion
}

func (f *fakeClearSessionStore) ListActiveSessionContexts(_ context.Context, _, _ string) ([]store.SessionAgent, error) {
	return f.agents, f.listErr
}

func (f *fakeClearSessionStore) DeleteSessionContextForEngine(_ context.Context, sessionKey, _, _ string) (bool, error) {
	f.deletedSessions = append(f.deletedSessions, sessionKey)
	return f.deleted, f.deleteErr
}

type fakeClearTaskStore struct {
	tasks     []model.Task
	listErr   error
	cancelled int64
	cancelErr error
}

func (f *fakeClearTaskStore) ListBySessionKey(_ context.Context, _, _, _ string) ([]model.Task, error) {
	return f.tasks, f.listErr
}

func (f *fakeClearTaskStore) CancelBySessionKey(_ context.Context, _, _ string) (int64, error) {
	return f.cancelled, f.cancelErr
}

type fakeClearWorkerLookup struct {
	byName map[string][]model.Worker
	byID   map[string]model.Worker
	err    error
}

func (f *fakeClearWorkerLookup) ListByName(name string) ([]model.Worker, error) {
	return f.byName[name], f.err
}

func (f *fakeClearWorkerLookup) GetByIDs(ids []string) ([]model.Worker, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.Worker, 0, len(ids))
	for _, id := range ids {
		if w, ok := f.byID[id]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

type fakeClearExecStopper struct {
	stopped []string
}

func (f *fakeClearExecStopper) StopExecution(executionID string) error {
	f.stopped = append(f.stopped, executionID)
	return nil
}

type fakeClearSessionDispatcher struct {
	cleared []string
}

func (f *fakeClearSessionDispatcher) ClearSession(sessionKey string) {
	f.cleared = append(f.cleared, sessionKey)
}

type clearFixture struct {
	handler  *command.ClearCommandHandler
	sender   *fakeSender
	sessions *fakeClearSessionStore
	tasks    *fakeClearTaskStore
	stopper  *fakeClearExecStopper
	disp     *fakeClearSessionDispatcher
}

func makeClearFixture(
	agents []store.SessionAgent,
	tasks []model.Task,
	workersByID map[string]model.Worker,
	clock time.Time,
) *clearFixture {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeClearSessionStore{agents: agents, deleted: true}
	taskStore := &fakeClearTaskStore{tasks: tasks}
	workers := &fakeClearWorkerLookup{byID: workersByID}
	stopper := &fakeClearExecStopper{}
	disp := &fakeClearSessionDispatcher{}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewClearCommandHandler(workers, sessions, taskStore, stopper, disp, senders, engineCfg)
	command.SetClearClockForTest(h, func() time.Time { return clock })
	return &clearFixture{
		handler:  h,
		sender:   sender,
		sessions: sessions,
		tasks:    taskStore,
		stopper:  stopper,
		disp:     disp,
	}
}

func TestClearCommand_ConfirmPromptListsTasksAndAgents(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "关羽", UpdatedAt: nowMs - 30*1000},
		{AgentID: "w2", AgentType: "worker", Engine: "claude", Name: "马超", UpdatedAt: nowMs - 30*1000},
		{AgentID: "bee", AgentType: "bee", Engine: "claude", Name: "bee", UpdatedAt: nowMs - 30*1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "帮我写一个排序算法", ExecutionID: "a1b2c3d4e5f6", CreatedAt: nowMs - 180*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
		{ID: "t2", WorkerID: "bee", Instruction: "总结今天的会议纪要", ExecutionID: "e5f6a7b89999", CreatedAt: nowMs - 12*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{
		"w1":  {ID: "w1", Name: "关羽"},
		"bee": {ID: "bee", Name: "bee"},
	}
	fx := makeClearFixture(agents, tasks, workers, clock)

	handled := fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	out := fx.sender.sent[0]

	for _, want := range []string{
		"⚠️ 将清除以下会话上下文：",
		"  - 关羽（claude）",
		"  - 马超（claude）",
		"  - bee（claude）",
		"同时将终止 2 个运行中任务：",
		"- [关羽] 帮我写一个排序算法   已运行 3m   exec: a1b2c3d4",
		"- [bee] 总结今天的会议纪要   已运行 12s   exec: e5f6a7b8",
		"30s 内再发一次 /clear 确认。",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}

	// Pending must be set; tasks must NOT have been stopped or cancelled yet.
	if len(fx.stopper.stopped) != 0 {
		t.Errorf("expected no executions stopped on first /clear, got %v", fx.stopper.stopped)
	}
	if len(fx.disp.cleared) != 0 {
		t.Errorf("expected no session cleared on first /clear, got %v", fx.disp.cleared)
	}
}

func TestClearCommand_ConfirmedWithin30sStopsAndClears(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "关羽", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "do work", ExecutionID: "exec-1234abcd", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "关羽"}}
	fx := makeClearFixture(agents, tasks, workers, clock)
	fx.tasks.cancelled = 1

	// First /clear -> confirmation prompt.
	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply after first /clear, got %d", len(fx.sender.sent))
	}

	// Second /clear within 30s -> actually stop & clear.
	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 2 {
		t.Fatalf("expected 2 replies total, got %d", len(fx.sender.sent))
	}
	if got, want := fx.stopper.stopped, []string{"exec-1234abcd"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("expected stopped=%v, got %v", want, got)
	}
	if len(fx.disp.cleared) != 1 {
		t.Errorf("expected ClearSession called once, got %d", len(fx.disp.cleared))
	}
	if !strings.Contains(fx.sender.sent[1], "✅ 已清除：") {
		t.Errorf("expected cleared message, got: %s", fx.sender.sent[1])
	}
}

func TestClearCommand_ConfirmPromptFallsBackToWorkerID(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "ghost", AgentType: "worker", Engine: "claude", Name: "ghost", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "ghost", Instruction: "do something", ExecutionID: "deadbeef0000", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	fx := makeClearFixture(agents, tasks, nil, clock)

	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	out := fx.sender.sent[0]
	if !strings.Contains(out, "[ghost] do something") {
		t.Errorf("expected raw worker id fallback in task line, got:\n%s", out)
	}
}
