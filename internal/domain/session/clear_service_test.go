package session_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

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

type fakeTaskStore struct {
	tasks      []model.Task
	listErr    error
	cancelled  int64
	cancelErr  error
	listCalls  []store.TaskFilter
	cancelCall []store.CancelFilter
}

func (f *fakeTaskStore) List(_ context.Context, fl store.TaskFilter) ([]model.Task, error) {
	f.listCalls = append(f.listCalls, fl)
	return f.tasks, f.listErr
}

func (f *fakeTaskStore) Cancel(_ context.Context, fl store.CancelFilter) (int64, error) {
	f.cancelCall = append(f.cancelCall, fl)
	return f.cancelled, f.cancelErr
}

type fakeExecStopper struct {
	mu      sync.Mutex
	stopped []string
	err     error
	delay   <-chan struct{}
}

func (f *fakeExecStopper) StopExecution(id string) error {
	if f.delay != nil {
		<-f.delay
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
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
}

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
		EngineCfg:     enginecfg.NewStore("claude"),
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
		EngineCfg:     enginecfg.NewStore("claude"),
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
