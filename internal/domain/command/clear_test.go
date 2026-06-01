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
	tasks             []model.Task
	listErr           error
	cancelled         int64
	cancelErr         error
	workerTasks       map[string][]model.Task // workerID → running tasks scoped to that worker
	workerCancelled   map[string]int64        // workerID → rows cancelled
	workerListCalls   []string                // captured (sessionKey+"::"+workerID)
	workerCancelCalls []string                // captured (sessionKey+"::"+workerID)
}

func (f *fakeClearTaskStore) List(_ context.Context, fl store.TaskFilter) ([]model.Task, error) {
	if fl.WorkerID != "" {
		f.workerListCalls = append(f.workerListCalls, fl.SessionKey+"::"+fl.WorkerID)
		return f.workerTasks[fl.WorkerID], f.listErr
	}
	return f.tasks, f.listErr
}

func (f *fakeClearTaskStore) Cancel(_ context.Context, fl store.CancelFilter) (int64, error) {
	if fl.WorkerID != "" {
		f.workerCancelCalls = append(f.workerCancelCalls, fl.SessionKey+"::"+fl.WorkerID)
		return f.workerCancelled[fl.WorkerID], f.cancelErr
	}
	return f.cancelled, f.cancelErr
}

type fakeClearWorkerLookup struct {
	*fakeWorkerByIDsLookup
	byName map[string][]model.Worker
}

func (f *fakeClearWorkerLookup) ListByName(name string) ([]model.Worker, error) {
	return f.byName[name], f.err
}

type fakeClearExecStopper struct {
	stopped []string
}

func (f *fakeClearExecStopper) StopExecution(executionID string) error {
	f.stopped = append(f.stopped, executionID)
	return nil
}

type fakeClearSessionDispatcher struct {
	cleared        []string
	clearedWorkers []string // captured (sessionKey+"::"+workerID)
}

func (f *fakeClearSessionDispatcher) ClearSession(sessionKey string) {
	f.cleared = append(f.cleared, sessionKey)
}

func (f *fakeClearSessionDispatcher) ClearWorker(sessionKey, workerID string) {
	f.clearedWorkers = append(f.clearedWorkers, sessionKey+"::"+workerID)
}

type clearFixture struct {
	handler  *command.ClearCommandHandler
	sender   *fakeSender
	sessions *fakeClearSessionStore
	tasks    *fakeClearTaskStore
	stopper  *fakeClearExecStopper
	disp     *fakeClearSessionDispatcher
}

type clearFixtureOpts struct {
	cancelled    int64
	clock        func() time.Time
	runningExecs fakeRunningExecs
}

type clearFixtureOpt func(*clearFixtureOpts)

func withClearClock(now func() time.Time) clearFixtureOpt {
	return func(o *clearFixtureOpts) { o.clock = now }
}

func withClearCancelled(n int64) clearFixtureOpt {
	return func(o *clearFixtureOpts) { o.cancelled = n }
}

func withClearRunningExecs(m fakeRunningExecs) clearFixtureOpt {
	return func(o *clearFixtureOpts) { o.runningExecs = m }
}

func makeClearFixture(
	agents []store.SessionAgent,
	tasks []model.Task,
	workersByID map[string]model.Worker,
	opts ...clearFixtureOpt,
) *clearFixture {
	cfg := &clearFixtureOpts{}
	for _, opt := range opts {
		opt(cfg)
	}
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeClearSessionStore{agents: agents, deleted: true}
	taskStore := &fakeClearTaskStore{tasks: tasks, cancelled: cfg.cancelled}
	workers := &fakeClearWorkerLookup{fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{byID: workersByID}}
	stopper := &fakeClearExecStopper{}
	disp := &fakeClearSessionDispatcher{}
	engineCfg := enginecfg.NewStore("claude")
	runningExecs := cfg.runningExecs
	if runningExecs == nil {
		runningExecs = fakeRunningExecs{}
	}
	h := command.NewClearCommandHandler(workers, sessions, taskStore, stopper, disp, senders, engineCfg, runningExecs)
	if cfg.clock != nil {
		command.SetClearClockForTest(h, cfg.clock)
	}
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
		{ID: "t1", WorkerID: "w1", Instruction: "帮我写一个排序算法", CreatedAt: nowMs - 180*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
		{ID: "t2", WorkerID: "bee", Instruction: "总结今天的会议纪要", CreatedAt: nowMs - 12*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{
		"w1":  {ID: "w1", Name: "关羽"},
		"bee": {ID: "bee", Name: "bee"},
	}
	fx := makeClearFixture(agents, tasks, workers, withClearClock(fixedClock(clock)),
		withClearRunningExecs(fakeRunningExecs{"t1": "a1b2c3d4e5f6", "t2": "e5f6a7b89999"}))

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
		{ID: "t1", WorkerID: "w1", Instruction: "do work", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "关羽"}}
	fx := makeClearFixture(agents, tasks, workers, withClearClock(fixedClock(clock)), withClearCancelled(1),
		withClearRunningExecs(fakeRunningExecs{"t1": "exec-1234abcd"}))

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

func TestClearCommand_ConfirmExpiresAfter30s(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "关羽", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "do work", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "关羽"}}
	current := clock
	fx := makeClearFixture(agents, tasks, workers, withClearClock(func() time.Time { return current }))

	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply after first /clear, got %d", len(fx.sender.sent))
	}

	current = clock.Add(31 * time.Second)
	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 2 {
		t.Fatalf("expected 2 replies total, got %d", len(fx.sender.sent))
	}
	if len(fx.stopper.stopped) != 0 {
		t.Errorf("expected no execution stopped after window expired, got %v", fx.stopper.stopped)
	}
	if len(fx.disp.cleared) != 0 {
		t.Errorf("expected no session cleared after window expired, got %v", fx.disp.cleared)
	}
	if !strings.Contains(fx.sender.sent[1], "30s 内再发一次 /clear 确认。") {
		t.Errorf("expected new confirm prompt after expiry, got: %s", fx.sender.sent[1])
	}
}

func TestClearCommand_Worker_NoRunningTasks_ClearsImmediately(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	fx := makeClearFixture(nil, nil, nil, withClearClock(fixedClock(clock)))
	fx.tasks.workerTasks = map[string][]model.Task{}
	fx.sessions.deleted = true
	worker := model.Worker{ID: "w1", Name: "徐晃", Engine: "claude"}
	workers := &fakeClearWorkerLookup{
		fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{byID: map[string]model.Worker{"w1": worker}},
		byName:                map[string][]model.Worker{"徐晃": {worker}},
	}
	engineCfg := enginecfg.NewStore("claude")
	senders := map[string]platform.PlatformSenderAdapter{"feishu": fx.sender}
	fx.handler = command.NewClearCommandHandler(workers, fx.sessions, fx.tasks, fx.stopper, fx.disp, senders, engineCfg, fakeRunningExecs{})
	command.SetClearClockForTest(fx.handler, fixedClock(clock))

	fx.handler.HandleCommand(context.Background(), "/clear 徐晃", makeReplyTo())

	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	if !strings.Contains(fx.sender.sent[0], "✅ 已清除 徐晃") {
		t.Errorf("expected worker_cleared message, got: %s", fx.sender.sent[0])
	}
	if len(fx.stopper.stopped) != 0 {
		t.Errorf("expected no executions stopped when no running tasks, got %v", fx.stopper.stopped)
	}
	if got := fx.disp.clearedWorkers; len(got) != 1 || got[0] != "feishu:chat1:user1::w1" {
		t.Errorf("expected ClearWorker(session, w1), got %v", got)
	}
	if got := fx.tasks.workerCancelCalls; len(got) != 1 || got[0] != "feishu:chat1:user1::w1" {
		t.Errorf("expected CancelBySessionAndWorker(session, w1), got %v", got)
	}
}

func TestClearCommand_Worker_WithRunningTasks_RequiresConfirm(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	fx := makeClearFixture(nil, nil, nil, withClearClock(fixedClock(clock)))
	fx.sessions.deleted = true
	worker := model.Worker{ID: "w1", Name: "徐晃", Engine: "claude"}
	fx.tasks.workerTasks = map[string][]model.Task{
		"w1": {{
			ID: "t1", WorkerID: "w1", Instruction: "do something",
			CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate,
		}},
	}
	fx.tasks.workerCancelled = map[string]int64{"w1": 1}
	workers := &fakeClearWorkerLookup{
		fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{byID: map[string]model.Worker{"w1": worker}},
		byName:                map[string][]model.Worker{"徐晃": {worker}},
	}
	engineCfg := enginecfg.NewStore("claude")
	senders := map[string]platform.PlatformSenderAdapter{"feishu": fx.sender}
	fx.handler = command.NewClearCommandHandler(workers, fx.sessions, fx.tasks, fx.stopper, fx.disp, senders, engineCfg, fakeRunningExecs{"t1": "exec-w1-task1"})
	command.SetClearClockForTest(fx.handler, fixedClock(clock))

	// First /clear 徐晃 → confirm prompt; no destructive action yet.
	fx.handler.HandleCommand(context.Background(), "/clear 徐晃", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply after first /clear, got %d", len(fx.sender.sent))
	}
	first := fx.sender.sent[0]
	for _, want := range []string{
		"⚠️ 将清除 徐晃",
		"同时将终止 1 个运行中任务",
		"[徐晃] do something",
		"30s 内再发一次 /clear 徐晃 确认。",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("confirm prompt missing %q, got:\n%s", want, first)
		}
	}
	if len(fx.stopper.stopped) != 0 {
		t.Errorf("expected no executions stopped on first /clear, got %v", fx.stopper.stopped)
	}
	if len(fx.disp.clearedWorkers) != 0 {
		t.Errorf("expected ClearWorker not yet called, got %v", fx.disp.clearedWorkers)
	}

	// Second /clear 徐晃 within 30s → execute the clear.
	fx.handler.HandleCommand(context.Background(), "/clear 徐晃", makeReplyTo())
	if len(fx.sender.sent) != 2 {
		t.Fatalf("expected 2 replies total, got %d", len(fx.sender.sent))
	}
	if got, want := fx.stopper.stopped, []string{"exec-w1-task1"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("expected stopped=%v, got %v", want, got)
	}
	if got := fx.disp.clearedWorkers; len(got) != 1 || got[0] != "feishu:chat1:user1::w1" {
		t.Errorf("expected ClearWorker(session, w1) once, got %v", got)
	}
	if !strings.Contains(fx.sender.sent[1], "✅ 已清除 徐晃") || !strings.Contains(fx.sender.sent[1], "取消了 1 个任务") {
		t.Errorf("expected worker_cleared_with_tasks message, got: %s", fx.sender.sent[1])
	}
}

func TestClearCommand_Worker_OnlyAffectsTargetWorker(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	fx := makeClearFixture(nil, nil, nil, withClearClock(fixedClock(clock)))
	fx.sessions.deleted = true
	xuhuang := model.Worker{ID: "w1", Name: "徐晃", Engine: "claude"}
	diaochan := model.Worker{ID: "w2", Name: "貂蝉", Engine: "claude"}
	fx.tasks.workerTasks = map[string][]model.Task{
		"w1": {{
			ID: "t1", WorkerID: "w1", Instruction: "a",
			CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate,
		}},
		"w2": {{
			ID: "t2", WorkerID: "w2", Instruction: "b",
			CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate,
		}},
	}
	fx.tasks.workerCancelled = map[string]int64{"w1": 1, "w2": 1}
	workers := &fakeClearWorkerLookup{
		fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{byID: map[string]model.Worker{"w1": xuhuang, "w2": diaochan}},
		byName: map[string][]model.Worker{
			"徐晃": {xuhuang},
			"貂蝉": {diaochan},
		},
	}
	engineCfg := enginecfg.NewStore("claude")
	senders := map[string]platform.PlatformSenderAdapter{"feishu": fx.sender}
	fx.handler = command.NewClearCommandHandler(workers, fx.sessions, fx.tasks, fx.stopper, fx.disp, senders, engineCfg, fakeRunningExecs{"t1": "exec-w1", "t2": "exec-w2"})
	command.SetClearClockForTest(fx.handler, fixedClock(clock))

	// First call → confirm prompt
	fx.handler.HandleCommand(context.Background(), "/clear 徐晃", makeReplyTo())
	// Second call → real clear
	fx.handler.HandleCommand(context.Background(), "/clear 徐晃", makeReplyTo())

	// Only 徐晃's exec should have been stopped — 貂蝉's must be untouched.
	if got, want := fx.stopper.stopped, []string{"exec-w1"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("expected only exec-w1 stopped, got %v", got)
	}
	for _, call := range fx.tasks.workerCancelCalls {
		if call == "feishu:chat1:user1::w2" {
			t.Errorf("貂蝉 must not have tasks cancelled, got cancel call %s", call)
		}
	}
	for _, call := range fx.disp.clearedWorkers {
		if call == "feishu:chat1:user1::w2" {
			t.Errorf("貂蝉 queue must not be cleared, got %s", call)
		}
	}
}

func TestClearCommand_ConfirmPromptFallsBackToWorkerID(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "ghost", AgentType: "worker", Engine: "claude", Name: "ghost", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "ghost", Instruction: "do something", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	fx := makeClearFixture(agents, tasks, nil, withClearClock(fixedClock(clock)))

	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	out := fx.sender.sent[0]
	if !strings.Contains(out, "[ghost] do something") {
		t.Errorf("expected raw worker id fallback in task line, got:\n%s", out)
	}
}
