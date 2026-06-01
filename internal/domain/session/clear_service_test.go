package session_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

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
	return session.NewClearService(sessions, tasks, stopper, finalizer, disp, execs, enginecfg.NewStore("claude"))
}

func TestClearSession_EmptySession_NoOp(t *testing.T) {
	sessions := &fakeSessionStore{agents: nil}
	tasks := &fakeTaskStore{tasks: nil}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	got, err := svc.ClearSession(context.Background(), "sess-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Cleared || len(got.Agents) != 0 || len(got.ActiveTasks) != 0 {
		t.Fatalf("expected no-op empty result, got %+v", got)
	}
	if len(disp.sessions) != 0 {
		t.Fatalf("dispatcher must not be signalled for empty session, got %v", disp.sessions)
	}
}

func TestClearSession_TasksWithoutAgents_GatesThenCleansOnForce(t *testing.T) {
	sessions := &fakeSessionStore{agents: nil}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	gated, err := svc.ClearSession(context.Background(), "sess-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gated.Cleared || len(gated.ActiveTasks) != 1 {
		t.Fatalf("expected gating on tasks even without agents, got %+v", gated)
	}

	cleared, err := svc.ClearSession(context.Background(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleared.Cleared || cleared.CancelledTasks != 1 {
		t.Fatalf("expected force to clean tasks without agents, got %+v", cleared)
	}
	if len(disp.sessions) != 1 || disp.sessions[0] != "sess-1" {
		t.Fatalf("expected ClearSession dispatcher call, got %v", disp.sessions)
	}
}

func TestClearSession_ActiveTasks_GatesWhenNotForced(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1", Engine: "claude"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.ClearSession(context.Background(), "sess-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Cleared || len(got.ActiveTasks) != 1 {
		t.Fatalf("expected gated result, got %+v", got)
	}
	if len(stopper.stopped) != 0 || len(disp.sessions) != 0 || len(tasks.cancelCall) != 0 {
		t.Fatal("destructive ops must not run when gated")
	}
}

func TestClearSession_Force_StopsAndClears(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1", Engine: "claude"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.ClearSession(context.Background(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Cleared || got.CancelledTasks != 1 {
		t.Fatalf("expected cleared result, got %+v", got)
	}
	if got, want := stopper.stopped, []string{"exec-1"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("expected stop exec-1, got %v", got)
	}
	if len(disp.sessions) != 1 || disp.sessions[0] != "sess-1" {
		t.Fatalf("expected dispatcher.ClearSession(sess-1), got %v", disp.sessions)
	}
}

func TestClearSession_StopFails_FinalizesExecution(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1"}}}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1"}}}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{err: errStopFailed}
	fin := &fakeExecFinalizer{}
	svc := newSvc(t, sessions, tasks, stopper, fin, disp, fakeRunningExecs{"t1": "exec-1"})

	if _, err := svc.ClearSession(context.Background(), "sess-1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fin.abandoned) != 1 || fin.abandoned[0] != "exec-1" {
		t.Fatalf("expected MarkAbandoned(exec-1), got %v", fin.abandoned)
	}
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

	done := make(chan struct{})
	go func() {
		_, _ = svc.ClearSession(context.Background(), "sess-1", true)
		close(done)
	}()

	// Concurrent: all three goroutines should block on the channel before we
	// release it. Sequential: only one would be blocked.
	close(release)
	<-done

	got := append([]string(nil), stopper.stopped...)
	sort.Strings(got)
	if want := []string{"exec-1", "exec-2", "exec-3"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("expected all three execs stopped, got %v", got)
	}
}

func TestClearSession_CancelError_ReturnsErrorAndSkipsDispatcher(t *testing.T) {
	sessions := &fakeSessionStore{agents: []store.SessionAgent{{AgentID: "w1"}}}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1"}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	result, err := svc.ClearSession(context.Background(), "sess-1", true)
	if err == nil {
		t.Fatalf("expected error when cancel fails, got nil; result=%+v", result)
	}
	if result.Cleared {
		t.Fatalf("Cleared must be false when cancel failed, got %+v", result)
	}
	if len(disp.sessions) != 0 {
		t.Fatalf("dispatcher.ClearSession must not run after cancel failure, got %v", disp.sessions)
	}
}

func TestClearWorker_CancelError_ReturnsErrorAndSkipsDispatcher(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{
		tasks:     []model.Task{{ID: "t1", WorkerID: "w1"}},
		cancelErr: errors.New("db down"),
	}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	result, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"}, true)
	if err == nil {
		t.Fatalf("expected error when cancel fails, got nil; result=%+v", result)
	}
	if result.Cleared {
		t.Fatalf("Cleared must be false when cancel failed, got %+v", result)
	}
	if len(disp.workers) != 0 {
		t.Fatalf("dispatcher.ClearWorker must not run after cancel failure, got %v", disp.workers)
	}
	if len(sessions.deletedCalls) != 0 {
		t.Fatalf("DeleteSessionContextForEngine must not run after cancel failure, got %v", sessions.deletedCalls)
	}
}

func TestClearWorker_GatesOnActiveTasks(t *testing.T) {
	sessions := &fakeSessionStore{}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Cleared || len(got.ActiveTasks) != 1 {
		t.Fatalf("expected gated result, got %+v", got)
	}
	if len(disp.workers) != 0 || len(sessions.deletedCalls) != 0 {
		t.Fatal("destructive ops must not run when gated")
	}
}

func TestClearWorker_Force_PerformsFullCleanup(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: []model.Task{{ID: "t1", WorkerID: "w1"}}, cancelled: 1}
	disp := &fakeDispatcher{}
	stopper := &fakeExecStopper{}
	svc := newSvc(t, sessions, tasks, stopper, &fakeExecFinalizer{}, disp, fakeRunningExecs{"t1": "exec-1"})

	got, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1", Engine: "claude"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Cleared || got.CancelledTasks != 1 || !got.DeletedContext {
		t.Fatalf("expected full cleanup, got %+v", got)
	}
	if len(stopper.stopped) != 1 || stopper.stopped[0] != "exec-1" {
		t.Fatalf("expected stop exec-1, got %v", stopper.stopped)
	}
	if got := sessions.deletedCalls; len(got) != 1 || got[0] != (deletedCall{"sess-1", "w1", "claude"}) {
		t.Fatalf("expected delete (sess-1, w1, claude), got %v", got)
	}
	if len(disp.workers) != 1 || disp.workers[0] != "sess-1::w1" {
		t.Fatalf("expected ClearWorker(sess-1, w1), got %v", disp.workers)
	}
}

func TestClearWorker_NoActiveTasks_StillDeletesContextAndClearsQueue(t *testing.T) {
	sessions := &fakeSessionStore{deleted: true}
	tasks := &fakeTaskStore{tasks: nil}
	disp := &fakeDispatcher{}
	svc := newSvc(t, sessions, tasks, &fakeExecStopper{}, &fakeExecFinalizer{}, disp, nil)

	got, err := svc.ClearWorker(context.Background(), "sess-1", model.Worker{ID: "w1"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Cleared {
		t.Fatalf("expected cleared, got %+v", got)
	}
	if len(disp.workers) != 1 {
		t.Fatalf("expected ClearWorker called once, got %v", disp.workers)
	}
}

var errStopFailed = stopErr("stop failed")

type stopErr string

func (e stopErr) Error() string { return string(e) }
