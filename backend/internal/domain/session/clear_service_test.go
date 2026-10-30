package session_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type fakeSessionStore struct {
	agents          []store.SessionAgent
	listErr         error
	deleted         bool
	deleteErr       error
	deletedCalls    []deletedCall
	listActiveCalls []listActiveCall
}

type listActiveCall struct {
	sessionKey string
	beeEngine  string
}

type deletedCall struct {
	sessionKey, agentID, engine string
}

func (f *fakeSessionStore) ListActiveSessionContexts(_ context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error) {
	f.listActiveCalls = append(f.listActiveCalls, listActiveCall{sessionKey, beeEngine})
	return f.agents, f.listErr
}

func (f *fakeSessionStore) DeleteSessionContextForEngine(_ context.Context, sessionKey, agentID, engine string) (bool, error) {
	f.deletedCalls = append(f.deletedCalls, deletedCall{sessionKey, agentID, engine})
	return f.deleted, f.deleteErr
}

type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(call string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

func (l *callLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

type fakeTaskStore struct {
	tasks      []model.Task
	byType     map[string][]model.Task
	listErr    error
	cancelled  int64
	cancelErr  error
	listCalls  []store.TaskFilter
	cancelCall []store.CancelFilter
	log        *callLog
}

func (f *fakeTaskStore) List(_ context.Context, fl store.TaskFilter) ([]model.Task, error) {
	f.listCalls = append(f.listCalls, fl)
	if f.byType != nil {
		return f.byType[fl.Type], f.listErr
	}
	return f.tasks, f.listErr
}

func (f *fakeTaskStore) Cancel(_ context.Context, fl store.CancelFilter) (int64, error) {
	f.cancelCall = append(f.cancelCall, fl)
	f.log.add("cancel " + fl.Type)
	return f.cancelled, f.cancelErr
}

type fakeExecStopper struct {
	mu      sync.Mutex
	stopped []string
	err     error
	delay   <-chan struct{}
	log     *callLog
}

func (f *fakeExecStopper) StopExecution(id string) error {
	if f.delay != nil {
		<-f.delay
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
	f.log.add("stop " + id)
	return f.err
}

type fakeExecFinalizer struct {
	mu        sync.Mutex
	abandoned []string
}

func (f *fakeExecFinalizer) MarkAbandoned(_ context.Context, id, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abandoned = append(f.abandoned, id)
	return true, nil
}

type fakeDispatcher struct {
	sessions []string
	workers  []string
	aborted  []string
}

func (f *fakeDispatcher) AbortRun(taskID string) { f.aborted = append(f.aborted, taskID) }

func (f *fakeDispatcher) ClearSession(sk string) { f.sessions = append(f.sessions, sk) }
func (f *fakeDispatcher) ClearWorker(sk, wid string) {
	f.workers = append(f.workers, sk+"::"+wid)
}

type fakeTaskCanceller struct {
	called []string
	err    error
}

func (f *fakeTaskCanceller) CancelTask(_ context.Context, taskID string) error {
	f.called = append(f.called, taskID)
	return f.err
}

type fakeRunningExecs map[string]string

func (f fakeRunningExecs) RunningExecIDsByTaskIDs(_ context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if v, ok := f[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func newSvc(t *testing.T, sessions *fakeSessionStore, tasks *fakeTaskStore, stopper *fakeExecStopper, finalizer *fakeExecFinalizer, disp *fakeDispatcher, execs fakeRunningExecs) *session.ClearService {
	t.Helper()
	return session.NewClearService(session.ClearServiceDeps{
		Sessions:      sessions,
		Tasks:         tasks,
		ExecStopper:   stopper,
		ExecFinalizer: finalizer,
		Dispatcher:    disp,
		TaskCanceller: &fakeTaskCanceller{},
		RunningExecs:  execs,
		EngineCfg:     enginecfg.NewStore("claude", "", aitest.IDs("claude")),
	})
}

func newSvcWithCanceller(t *testing.T, sessions *fakeSessionStore, tasks *fakeTaskStore, stopper *fakeExecStopper, finalizer *fakeExecFinalizer, disp *fakeDispatcher, execs fakeRunningExecs, canceller *fakeTaskCanceller) *session.ClearService {
	t.Helper()
	return session.NewClearService(session.ClearServiceDeps{
		Sessions:      sessions,
		Tasks:         tasks,
		ExecStopper:   stopper,
		ExecFinalizer: finalizer,
		Dispatcher:    disp,
		TaskCanceller: canceller,
		RunningExecs:  execs,
		EngineCfg:     enginecfg.NewStore("claude", "", aitest.IDs("claude")),
	})
}

func TestEvaluateClearSession_EmptySession(t *testing.T) {
	sessions := &fakeSessionStore{agents: nil}
	tasks := &fakeTaskStore{tasks: nil}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, &fakeDispatcher{}, nil)

	got, err := svc.EvaluateClearSession(context.Background(), "sess-1")
	require.NoError(t, err)
	require.Empty(t, got.Agents)
	require.Empty(t, got.ActiveTasks)
}

func TestEvaluateClearSession_ReturnsAgentsAndTasks(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1", Engine: "claude"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate}}}
	stopper := &fakeExecStopper{}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.EvaluateClearSession(context.Background(), "sess-1")
	require.NoError(t, err)
	require.Len(t, got.Agents, 1)
	require.Len(t, got.ActiveTasks, 1)
	require.Empty(t, stopper.stopped, "Evaluate must not run destructive ops")
	require.Empty(t, disp.sessions, "Evaluate must not run destructive ops")
	require.Empty(t, tasks.cancelCall, "Evaluate must not run destructive ops")
}

func TestClearSession_StopsAndClears(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1", Engine: "claude"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	preview := session.ClearSessionPreview{
		Agents:      []store.SessionAgent{{AgentID: "w1", Engine: "claude"}},
		ActiveTasks: []model.Task{{ID: "t1"}},
	}
	got, err := svc.ClearSession(context.Background(), "sess-1", preview)
	require.NoError(t, err)
	require.EqualValues(t, 1, got.CancelledTasks)
	require.Len(t, got.Agents, 1)
	require.Equal(t, []string{"exec-1"}, stopper.stopped)
	require.Equal(t, []string{"sess-1"}, disp.sessions)
}

func TestClearSession_StopFails_FinalizesExecution(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1"}}}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{err: errStopFailed}
	fin := &fakeExecFinalizer{}
	svc := newSvc(t, sessions, tasks, stopper, fin, disp, fakeRunningExecs{"t1": "exec-1"})

	preview := session.ClearSessionPreview{
		Agents:      []store.SessionAgent{{AgentID: "w1"}},
		ActiveTasks: []model.Task{{ID: "t1"}},
	}
	_, err := svc.ClearSession(context.Background(), "sess-1", preview)
	require.NoError(t, err)
	require.Equal(t, []string{"exec-1"}, fin.abandoned)
}

func TestClearSession_StopsConcurrently(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1"}}}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}},
		cancelled: 3,
	}
	disp := &fakeDispatcher{}
	release := make(chan struct{})
	stopper := &fakeExecStopper{delay: release}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{
		"t1": "exec-1", "t2": "exec-2", "t3": "exec-3",
	})

	preview := session.ClearSessionPreview{
		Agents:      []store.SessionAgent{{AgentID: "w1"}},
		ActiveTasks: []model.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}},
	}
	done := make(chan struct{})
	go func() {
		_, _ = svc.ClearSession(context.Background(), "sess-1", preview)
		close(done)
	}()

	// Concurrent: all three goroutines should block on the channel before we
	// release it. Sequential: only one would be blocked.
	close(release)
	<-done

	got := append([]string(nil), stopper.stopped...)
	sort.Strings(got)
	require.Equal(t, []string{"exec-1", "exec-2", "exec-3"}, got)
}

func TestClearSession_CancelError_ReturnsErrorAndSkipsDispatcher(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1"}}}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1"}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	preview := session.ClearSessionPreview{
		Agents:      []store.SessionAgent{{AgentID: "w1"}},
		ActiveTasks: []model.Task{{ID: "t1"}},
	}
	_, err := svc.ClearSession(context.Background(), "sess-1", preview)
	require.Error(t, err)
	require.Empty(t, disp.sessions, "dispatcher.ClearSession must not run after cancel failure")
}

func TestEvaluateClearWorker_ReturnsEngineAndTasks(t *testing.T) {
	sessions := &fakeSessionStore{}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}}
	stopper := &fakeExecStopper{}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.EvaluateClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"})
	require.NoError(t, err)
	require.Equal(t, "claude", got.Engine)
	require.Len(t, got.ActiveTasks, 1)
	require.Empty(t, disp.workers, "Evaluate must not run destructive ops")
	require.Empty(t, sessions.deletedCalls, "Evaluate must not run destructive ops")
	require.Empty(t, stopper.stopped, "Evaluate must not run destructive ops")
}

func TestClearWorker_CancelError_ReturnsErrorAndSkipsDispatcher(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1", WorkerID: "w1"}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	preview := session.ClearWorkerPreview{Engine: "claude", ActiveTasks: []model.Task{{ID: "t1", WorkerID: "w1"}}}
	_, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"}, preview)
	require.Error(t, err)
	require.Empty(t, disp.workers, "dispatcher.ClearWorker must not run after cancel failure")
	require.Empty(t, sessions.deletedCalls, "DeleteSessionContextForEngine must not run after cancel failure")
}

func TestClearWorker_PerformsFullCleanup(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	preview := session.ClearWorkerPreview{Engine: "claude", ActiveTasks: []model.Task{{ID: "t1", WorkerID: "w1"}}}
	got, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"}, preview)
	require.NoError(t, err)
	require.EqualValues(t, 1, got.CancelledTasks)
	require.True(t, got.DeletedContext)
	require.Equal(t, "claude", got.Engine)
	require.Equal(t, []string{"exec-1"}, stopper.stopped)
	require.Equal(t, []deletedCall{{"sess-1", "w1", "claude"}}, sessions.deletedCalls)
	require.Equal(t, []string{"sess-1::w1"}, disp.workers)
}

func TestClearWorker_NoActiveTasks_StillDeletesContextAndClearsQueue(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: nil}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	got, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1"}, session.ClearWorkerPreview{})
	require.NoError(t, err)
	require.True(t, got.DeletedContext, "expected DeletedContext=true")
	require.Len(t, disp.workers, 1)
}

func TestStopWorker_StopsAndCancels_WithoutDeletingContext(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.StopWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"})
	require.NoError(t, err)
	require.EqualValues(t, 1, got.CancelledTasks)
	require.Equal(t, []string{"exec-1"}, stopper.stopped)
	require.Equal(t, []string{"sess-1::w1"}, disp.workers)
	// The defining difference from ClearWorker: context is preserved.
	require.Empty(t, sessions.deletedCalls, "StopWorker must NOT delete session context")
}

func TestStopWorker_CancelError_ReturnsErrorAndSkipsDispatcher(t *testing.T) {
	sessions := &fakeSessionStore{}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1", WorkerID: "w1"}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	_, err := svc.StopWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"})
	require.Error(t, err)
	require.Empty(t, disp.workers, "dispatcher.ClearWorker must not run after cancel failure")
	require.Empty(t, sessions.deletedCalls, "StopWorker must never delete context")
}

func TestStopWorkerFromBeeChatTargetsWorkerChat(t *testing.T) {
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1", SessionKey: "local:u1:worker:w1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.StopWorker(context.Background(), "local:u1", model.Worker{ID: "w1", Engine: "claude"})
	require.NoError(t, err)
	require.EqualValues(t, 1, got.CancelledTasks)
	require.Len(t, tasks.listCalls, 1)
	require.Equal(t, "local:u1:worker:w1", tasks.listCalls[0].SessionKey)
	require.Equal(t, []store.CancelFilter{{SessionKey: "local:u1:worker:w1", WorkerID: "w1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Equal(t, []string{"local:u1:worker:w1::w1"}, disp.workers)
	require.Equal(t, []string{"exec-1"}, stopper.stopped)
}

func TestStopWorkerFromWorkerChatKeepsKey(t *testing.T) {
	tasks := &fakeTaskStore{}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.StopWorker(context.Background(), "local:u1:worker:w1", model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.Equal(t, []store.CancelFilter{{SessionKey: "local:u1:worker:w1", WorkerID: "w1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Equal(t, []string{"local:u1:worker:w1::w1"}, disp.workers)
}

func TestStopWorkerTelegramUnchanged(t *testing.T) {
	const key = "telegram:chat1:userA"
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1", SessionKey: key}}, cancelled: 1}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.StopWorker(context.Background(), key, model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.Len(t, tasks.listCalls, 1)
	require.Equal(t, key, tasks.listCalls[0].SessionKey)
	require.Equal(t, []store.CancelFilter{{SessionKey: key, WorkerID: "w1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Equal(t, []string{key + "::w1"}, disp.workers)
}

func TestEvaluateAndClearWorkerFromBeeChatTargetWorkerChat(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)
	w := model.Worker{ID: "w1", Engine: "claude"}

	preview, err := svc.EvaluateClearWorker(context.Background(), "local:u1", w)
	require.NoError(t, err)
	require.Len(t, tasks.listCalls, 1)
	require.Equal(t, "local:u1:worker:w1", tasks.listCalls[0].SessionKey)

	_, err = svc.ClearWorker(context.Background(), "local:u1", w, preview)
	require.NoError(t, err)
	require.Equal(t, []store.CancelFilter{{SessionKey: "local:u1:worker:w1", WorkerID: "w1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Equal(t, []deletedCall{{"local:u1:worker:w1", "w1", "claude"}}, sessions.deletedCalls)
	require.Equal(t, []string{"local:u1:worker:w1::w1"}, disp.workers)
}

func TestClearSessionFromBeeChatKeepsBeeKey(t *testing.T) {
	tasks := &fakeTaskStore{}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.ClearSession(context.Background(), "local:u1", session.ClearSessionPreview{})
	require.NoError(t, err)
	require.Equal(t, []store.CancelFilter{{SessionKey: "local:u1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Equal(t, []string{"local:u1"}, disp.sessions)
}

func TestStopConversationAbortsScheduledRunKeepsSchedule(t *testing.T) {
	tasks := &fakeTaskStore{
		byType: map[string][]model.Task{
			model.TaskTypeImmediate: {{ID: "imm1", WorkerID: "w1"}},
			"countdown,scheduled": {
				{ID: "sched1", WorkerID: "w1", Type: model.TaskTypeScheduled, Status: model.TaskStatusRunning},
				{ID: "count1", WorkerID: "w1", Type: model.TaskTypeCountdown, Status: model.TaskStatusRunning},
			},
		},
		cancelled: 1,
	}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, stopper, &fakeExecFinalizer{}, disp,
		fakeRunningExecs{"imm1": "exec-imm", "sched1": "exec-sched", "count1": "exec-count"})

	got, err := svc.StopConversation(context.Background(), "local:u1", model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.EqualValues(t, 1, got.CancelledTasks)

	require.Equal(t, []string{"sched1", "count1"}, disp.aborted)
	require.Equal(t, []string{"exec-imm"}, stopper.stopped, "scheduled runs are stopped by AbortRun, never by the service")
	require.Equal(t, []store.CancelFilter{{SessionKey: "local:u1:worker:w1", WorkerID: "w1", Type: model.TaskTypeImmediate}}, tasks.cancelCall)
	require.Contains(t, tasks.listCalls, store.TaskFilter{
		SessionKey: "local:u1:worker:w1",
		WorkerID:   "w1",
		Status:     model.TaskStatusActive,
		Type:       "countdown,scheduled",
	})
	require.Equal(t, []string{"local:u1:worker:w1::w1"}, disp.workers)
}

func TestStopConversationOnWorkerChatKey(t *testing.T) {
	tasks := &fakeTaskStore{
		byType: map[string][]model.Task{
			"countdown,scheduled": {{ID: "sched1", WorkerID: "w1", Type: model.TaskTypeScheduled}},
		},
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.StopConversation(context.Background(), "local:u1:worker:w1", model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.Equal(t, []string{"sched1"}, disp.aborted)
	require.Contains(t, tasks.listCalls, store.TaskFilter{
		SessionKey: "local:u1:worker:w1",
		WorkerID:   "w1",
		Status:     model.TaskStatusActive,
		Type:       "countdown,scheduled",
	})
}

func TestStopConversationStopWorkerErrorSkipsAbort(t *testing.T) {
	tasks := &fakeTaskStore{
		byType:    map[string][]model.Task{"countdown,scheduled": {{ID: "sched1", WorkerID: "w1"}}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.StopConversation(context.Background(), "local:u1", model.Worker{ID: "w1"})
	require.Error(t, err)
	require.Empty(t, disp.aborted)
}

func TestStopConversationListErrorReturnsError(t *testing.T) {
	tasks := &fakeTaskStore{listErr: errors.New("db down")}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	_, err := svc.StopConversation(context.Background(), "local:u1", model.Worker{ID: "w1"})
	require.Error(t, err)
	require.Empty(t, disp.aborted)
}

func TestStopConversationCancelsLocalTasksBeforeKillingThem(t *testing.T) {
	calls := &callLog{}
	tasks := &fakeTaskStore{
		byType:    map[string][]model.Task{model.TaskTypeImmediate: {{ID: "t1", WorkerID: "w1"}}},
		cancelled: 1,
		log:       calls,
	}
	stopper := &fakeExecStopper{log: calls}
	svc := newSvc(t, &fakeSessionStore{}, tasks, stopper, &fakeExecFinalizer{}, &fakeDispatcher{}, fakeRunningExecs{"t1": "exec-1"})

	_, err := svc.StopConversation(context.Background(), "local:u1:worker:w1", model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.Equal(t, []string{"cancel " + model.TaskTypeImmediate, "stop exec-1"}, calls.snapshot())
}

func TestClearWorkerCancelsLocalTasksBeforeKillingThem(t *testing.T) {
	calls := &callLog{}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}, cancelled: 1, log: calls}
	stopper := &fakeExecStopper{log: calls}
	svc := newSvc(t, &fakeSessionStore{deleted: true}, tasks, stopper, &fakeExecFinalizer{}, &fakeDispatcher{}, fakeRunningExecs{"t1": "exec-1"})
	w := model.Worker{ID: "w1", Engine: "claude"}

	preview, err := svc.EvaluateClearWorker(context.Background(), "local:u1", w)
	require.NoError(t, err)
	_, err = svc.ClearWorker(context.Background(), "local:u1", w, preview)
	require.NoError(t, err)
	require.Equal(t, []string{"cancel " + model.TaskTypeImmediate, "stop exec-1"}, calls.snapshot())
}

func TestStopConversationCancelErrorKillsNothing(t *testing.T) {
	tasks := &fakeTaskStore{
		byType:    map[string][]model.Task{model.TaskTypeImmediate: {{ID: "t1", WorkerID: "w1"}}},
		cancelErr: errors.New("db down"),
	}
	stopper := &fakeExecStopper{}
	disp := &fakeDispatcher{}
	svc := newSvc(t, &fakeSessionStore{}, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	_, err := svc.StopConversation(context.Background(), "local:u1:worker:w1", model.Worker{ID: "w1"})
	require.Error(t, err)
	require.Empty(t, stopper.stopped)
	require.Empty(t, disp.workers)
}

func TestStopWorkerTelegramKillsBeforeCancelling(t *testing.T) {
	const key = "telegram:chat1:userA"
	calls := &callLog{}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1", SessionKey: key}}, cancelled: 1, log: calls}
	stopper := &fakeExecStopper{log: calls}
	svc := newSvc(t, &fakeSessionStore{}, tasks, stopper, &fakeExecFinalizer{}, &fakeDispatcher{}, fakeRunningExecs{"t1": "exec-1"})

	_, err := svc.StopWorker(context.Background(), key, model.Worker{ID: "w1"})
	require.NoError(t, err)
	require.Equal(t, []string{"stop exec-1", "cancel " + model.TaskTypeImmediate}, calls.snapshot())
}

func TestCancelTask_StopsAndFinalizesOnError(t *testing.T) {
	stopper := &fakeExecStopper{err: errStopFailed}
	fin := &fakeExecFinalizer{}
	canceller := &fakeTaskCanceller{}
	execs := fakeRunningExecs{"task-1": "exec-1"}
	svc := newSvcWithCanceller(t, &fakeSessionStore{}, &fakeTaskStore{}, stopper, fin, &fakeDispatcher{}, execs, canceller)

	err := svc.CancelTask(context.Background(), "task-1")
	require.NoError(t, err)

	require.Equal(t, []string{"exec-1"}, stopper.stopped)
	require.Equal(t, []string{"exec-1"}, fin.abandoned)
	require.Equal(t, []string{"task-1"}, canceller.called)
}

func TestCancelTask_NoRunningExecutionStillCancels(t *testing.T) {
	stopper := &fakeExecStopper{}
	fin := &fakeExecFinalizer{}
	canceller := &fakeTaskCanceller{}
	svc := newSvcWithCanceller(t, &fakeSessionStore{}, &fakeTaskStore{}, stopper, fin, &fakeDispatcher{}, nil, canceller)

	err := svc.CancelTask(context.Background(), "task-1")
	require.NoError(t, err)

	require.Empty(t, stopper.stopped)
	require.Empty(t, fin.abandoned)
	require.Equal(t, []string{"task-1"}, canceller.called)
}

var errStopFailed = stopErr("stop failed")

type stopErr string

func (e stopErr) Error() string { return string(e) }
