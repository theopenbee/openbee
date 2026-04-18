package task_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- Mocks ---

type mockExecManager struct {
	mu                   sync.Mutex
	execResult           model.WorkerExecution
	resumedWithSessionID string
	executedInstructions []string
}

func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string, resume bool) (model.WorkerExecution, error) {
	m.mu.Lock()
	if resume {
		m.resumedWithSessionID = sessionID
	}
	m.executedInstructions = append(m.executedInstructions, instruction)
	m.mu.Unlock()
	return m.execResult, nil
}

func (m *mockExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

type mockExecutionQuerier struct {
	result model.WorkerExecution
}

func (m *mockExecutionQuerier) GetByID(_ string) (model.WorkerExecution, error) {
	return m.result, nil
}

type mockTaskStore struct {
	mu             sync.Mutex
	failedTasks    []string
	completedTasks []string
}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (s *mockTaskStore) FailTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedTasks = append(s.failedTasks, taskID)
	return nil
}
func (s *mockTaskStore) CompleteTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTasks = append(s.completedTasks, taskID)
	return nil
}
func (s *mockTaskStore) CancelTask(_ context.Context, taskID string) error { return nil }

type mockSessionStore struct {
	mu      sync.Mutex
	data    map[mockSessionRef]string
	cleared []string
	deleted []mockSessionRef
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{data: make(map[mockSessionRef]string)}
}

type mockSessionRef struct {
	sessionKey string
	agentID    string
	engine     string
}

func normalizeMockEngine(engine string) string {
	if engine == "" {
		return ai.EngineClaude
	}
	return engine
}

func newMockSessionRef(sessionKey, agentID, engine string) mockSessionRef {
	return mockSessionRef{
		sessionKey: sessionKey,
		agentID:    agentID,
		engine:     normalizeMockEngine(engine),
	}
}

func (s *mockSessionStore) GetSessionContextForEngine(_ context.Context, sessionKey, agentID, engine string) (sessionID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[newMockSessionRef(sessionKey, agentID, engine)], nil
}
func (s *mockSessionStore) UpsertSessionContext(_ context.Context, sessionKey, agentID, sessionID, engine string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[newMockSessionRef(sessionKey, agentID, engine)] = sessionID
	return nil
}
func (s *mockSessionStore) DeleteSessionContextForEngine(_ context.Context, sessionKey, agentID, engine string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := newMockSessionRef(sessionKey, agentID, engine)
	delete(s.data, ref)
	s.deleted = append(s.deleted, ref)
	return nil
}
func (s *mockSessionStore) ClearSessionContexts(_ context.Context, sessionKey, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleared = append(s.cleared, sessionKey)
	for ref := range s.data {
		if ref.sessionKey == sessionKey {
			delete(s.data, ref)
		}
	}
	return nil
}

func (s *mockSessionStore) sessionID(sessionKey, agentID, engine string) string {
	sessionID, _ := s.GetSessionContextForEngine(context.Background(), sessionKey, agentID, engine)
	return sessionID
}

func (s *mockSessionStore) deleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.deleted)
}

func (s *mockSessionStore) deletedRef(index int) (mockSessionRef, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.deleted) {
		return mockSessionRef{}, false
	}
	return s.deleted[index], true
}

type mockFailureNotifier struct {
	mu    sync.Mutex
	calls []failureCall
}

type failureCall struct {
	messageID string
	info      model.FailureInfo
}

func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, failureCall{messageID: messageID, info: info})
	return nil
}

func (n *mockFailureNotifier) waitForCall(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		count := len(n.calls)
		n.mu.Unlock()
		if count > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

type mockWorkerLookup struct {
	worker model.Worker
	err    error
}

func (m *mockWorkerLookup) GetByID(_ string) (model.Worker, error) {
	return m.worker, m.err
}

// orderedMockManager records whether UpsertSessionContext was called before ExecuteWorker.
type orderedMockManager struct {
	mu             sync.Mutex
	callOrder      []string // "upsert" or "execute"
	executed       atomic.Int64
	execResult     model.WorkerExecution
	receivedResume bool
	receivedSessID string
}

func (m *orderedMockManager) ExecuteWorker(_ context.Context, _, _, sessionID string, resume bool) (model.WorkerExecution, error) {
	m.mu.Lock()
	m.callOrder = append(m.callOrder, "execute")
	m.receivedResume = resume
	m.receivedSessID = sessionID
	m.mu.Unlock()
	m.executed.Add(1)
	return m.execResult, nil
}

func (m *orderedMockManager) CancelExecution(_ context.Context, _ string) error { return nil }

// orderedMockSessionStore wraps mockSessionStore and records upsert calls.
type orderedMockSessionStore struct {
	*mockSessionStore
	outer *orderedMockManager
}

func (s *orderedMockSessionStore) UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID, engine string) error {
	s.outer.mu.Lock()
	s.outer.callOrder = append(s.outer.callOrder, "upsert")
	s.outer.mu.Unlock()
	return s.mockSessionStore.UpsertSessionContext(ctx, sessionKey, agentID, sessionID, engine)
}

func newTaskDispatcher(mgr task.ExecutionManager, eq task.ExecutionQuerier, ss task.SessionStore, opts ...task.Option) (*task.TaskDispatcher, chan task.DispatchTask, *mockTaskStore) {
	in := make(chan task.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task.New(mgr, ts, ss, eq, in, opts...)
	return d, in, ts
}

func immediateTask(sessionKey, workerID, instruction string) task.DispatchTask {
	return task.DispatchTask{
		TaskID:      "task-1",
		WorkerID:    workerID,
		SessionKey:  sessionKey,
		Instruction: instruction,
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: sessionKey},
		TaskType:    "immediate",
		MessageID:   "msg-1",
	}
}

// waitFor polls until count() >= n or timeout elapses.
func waitFor(count func() int, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if count() >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// waitForExecCount waits until mgr.executedInstructions reaches n or timeout.
func waitForExecCount(mgr *mockExecManager, n int, timeout time.Duration) bool {
	return waitFor(func() int {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		return len(mgr.executedInstructions)
	}, n, timeout)
}

// --- Tests ---

func TestTaskDispatcher_ImmediateTask_CallsExecuteWorker(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted, Result: "done!"}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "check weather")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}
}

func TestTaskDispatcher_InstructionInjection(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task.DispatchTask{
		TaskID:      "task-abc",
		WorkerID:    "w1",
		SessionKey:  "s1",
		Instruction: "do the thing",
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: "s1"},
		TaskType:    "immediate",
		MessageID:   "msg-xyz",
	}
	in <- task

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	wantMeta := `<task_meta>{"message_id":"msg-xyz","task_id":"task-abc"}</task_meta>`
	if !strings.Contains(instr, wantMeta) {
		t.Errorf("instruction missing task_meta, got: %q", instr)
	}
	if !strings.Contains(instr, "<task_content>") {
		t.Errorf("instruction missing task_content tag, got: %q", instr)
	}
	if !strings.Contains(instr, "</task_content>") {
		t.Errorf("instruction missing closing task_content tag, got: %q", instr)
	}
	if !strings.Contains(instr, "do the thing") {
		t.Errorf("instruction missing original text, got: %q", instr)
	}
}

func TestTaskDispatcher_ClearSession_ClearsSessionContexts(t *testing.T) {
	ss := newMockSessionStore()
	d, _, _ := newTaskDispatcher(&mockExecManager{}, &mockExecutionQuerier{}, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	d.ClearSession("s1")

	// Wait for async clear
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		ss.mu.Lock()
		cleared := len(ss.cleared)
		ss.mu.Unlock()
		if cleared > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if len(ss.cleared) == 0 || ss.cleared[0] != "s1" {
		t.Errorf("expected ClearSessionContexts called with s1, got %v", ss.cleared)
	}
}

func TestTaskDispatcher_ClearSession_ClearsQueueAndSessionContexts(t *testing.T) {
	ss := newMockSessionStore()
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}

	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted, Result: "ok"}}
	in := make(chan task.DispatchTask, 4)
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// Send a task to create a queue entry
	t1 := immediateTask("s1", "w1", "first")
	t1.TaskID = "task-1"
	in <- t1

	// Wait for first task to start
	time.Sleep(50 * time.Millisecond)

	// Queue a second task (pending in queue)
	t2 := immediateTask("s1", "w1", "second")
	t2.TaskID = "task-2"
	in <- t2
	time.Sleep(20 * time.Millisecond)

	// Call ClearSession — should clear the pending queue entry and session contexts
	d.ClearSession("s1")
	time.Sleep(50 * time.Millisecond)

	// Unblock the first task
	close(blocker)
	time.Sleep(100 * time.Millisecond)

	// Session contexts should have been cleared
	ss.mu.Lock()
	cleared := ss.cleared
	ss.mu.Unlock()
	if len(cleared) == 0 || cleared[0] != "s1" {
		t.Errorf("expected ClearSessionContexts called with s1, got %v", cleared)
	}

	// Second task should NOT have executed (queue was cleared)
	if atomic.LoadInt64(&mgr.completed) > 1 {
		t.Errorf("expected at most 1 execution (second should be cleared from queue), got %d", atomic.LoadInt64(&mgr.completed))
	}
}

func TestTaskDispatcher_ImmediateTask_ResumesWhenSessionExists(t *testing.T) {
	ss := newMockSessionStore()
	_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "prior-session-id", "claude")

	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted, Result: "resumed!"}}
	enginecfg.Set("claude")
	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "follow-up")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorkerWithSession was not called within timeout")
	}

	mgr.mu.Lock()
	resumed := mgr.resumedWithSessionID
	mgr.mu.Unlock()

	if resumed != "prior-session-id" {
		t.Errorf("expected ExecuteWorkerWithSession with prior-session-id, got %q", resumed)
	}
}

func TestTaskDispatcher_ImmediateTask_EngineSwitch_PreservesPriorSession(t *testing.T) {
	ss := newMockSessionStore()
	_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "claude-session-id", "claude")

	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "codex-session-id"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", SessionID: "codex-session-id", Status: model.ExecStatusCompleted, Result: "fresh!"}}
	enginecfg.Set("codex")
	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "switch engine")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	resumed := mgr.resumedWithSessionID
	mgr.mu.Unlock()
	if resumed != "" {
		t.Errorf("expected fresh start on engine switch, got resume session %q", resumed)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ss.sessionID("s1", "w1", "codex") == "codex-session-id" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := ss.sessionID("s1", "w1", "claude"); got != "claude-session-id" {
		t.Errorf("expected claude session preserved, got %q", got)
	}
	if got := ss.sessionID("s1", "w1", "codex"); got != "codex-session-id" {
		t.Errorf("expected codex session stored, got %q", got)
	}
}

func TestTaskDispatcher_ImmediateTask_FreshWhenNoSession(t *testing.T) {
	ss := newMockSessionStore()
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "new-session"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted, Result: "fresh!"}}
	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "first message")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	resumed := mgr.resumedWithSessionID
	mgr.mu.Unlock()

	if resumed != "" {
		t.Errorf("expected fresh start (no resume), but ExecuteWorkerWithSession was called with %q", resumed)
	}
}

func TestTaskDispatcher_ImmediateTask_ResumeFails_FallsBackToFresh(t *testing.T) {
	ss := newMockSessionStore()
	_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "claude-session-id", "claude")
	_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "broken-session-id", "codex")

	mgr := &fallbackExecManager{
		freshResult: model.WorkerExecution{ID: "exec-fresh", SessionID: "new-session"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-fresh", SessionID: "new-session", Status: model.ExecStatusCompleted, Result: "fallback-ok"}}

	in := make(chan task.DispatchTask, 4)
	enginecfg.Set("codex")
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "message after broken session")

	// Wait for execution to complete — mgr.execCount reaches 1 (fresh execute)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&mgr.freshCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.freshCount) < 1 {
		t.Fatal("fallback ExecuteWorker was never called")
	}

	// Stale codex session should be deleted before the fresh run is started.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ss.deleteCount() > 0 && ss.sessionID("s1", "w1", "codex") == "new-session" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	deletedRef, ok := ss.deletedRef(0)
	if !ok || deletedRef != newMockSessionRef("s1", "w1", "codex") {
		t.Errorf("expected codex session deleted after resume failure, got %v", ss.deleted)
	}
	if len(ss.cleared) != 0 {
		t.Errorf("did not expect full session clear on resume failure, got %v", ss.cleared)
	}
	if got := ss.sessionID("s1", "w1", "claude"); got != "claude-session-id" {
		t.Errorf("expected claude session preserved, got %q", got)
	}
	if got := ss.sessionID("s1", "w1", "codex"); got != "new-session" {
		t.Errorf("expected codex session refreshed after fallback, got %q", got)
	}
}

func TestTaskDispatcher_TwoTasks_SameSession_Serialized(t *testing.T) {
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted, Result: "ok"}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "first")
	t1.TaskID = "task-1"
	t2 := immediateTask("s1", "w1", "second")
	t2.TaskID = "task-2"

	in <- t1
	in <- t2

	// Wait for first task to start blocking
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt64(&mgr.started) != 1 {
		t.Fatalf("expected 1 execution started, got %d", atomic.LoadInt64(&mgr.started))
	}

	// Unblock first execution
	close(blocker)

	// Wait for both to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&mgr.completed) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.completed) < 2 {
		t.Errorf("expected 2 executions completed, got %d", atomic.LoadInt64(&mgr.completed))
	}
}

func TestTaskDispatcher_CrossSession_SameWorker_Serialized(t *testing.T) {
	// Two different sessions both dispatch to the same worker.
	// They must execute sequentially — never concurrently.
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// Session s1 dispatches to worker w1
	t1 := immediateTask("s1", "w1", "from-s1")
	t1.TaskID = "task-s1"
	// Session s2 also dispatches to worker w1
	t2 := immediateTask("s2", "w1", "from-s2")
	t2.TaskID = "task-s2"

	in <- t1
	in <- t2

	// Wait for first task to start
	time.Sleep(50 * time.Millisecond)

	// Only one execution should have started — the second must be queued
	if atomic.LoadInt64(&mgr.started) != 1 {
		t.Fatalf("expected exactly 1 execution started (second should be queued), got %d", atomic.LoadInt64(&mgr.started))
	}

	// Unblock the first execution
	close(blocker)

	// Both should eventually complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&mgr.completed) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.completed) < 2 {
		t.Errorf("expected both tasks to complete, only %d completed", atomic.LoadInt64(&mgr.completed))
	}
}

func TestQueueKey_IgnoresSessionKey(t *testing.T) {
	// Same workerID, different sessionKeys must produce the same key.
	// This is the contract that prevents cross-session concurrent execution.
	k1 := task.ExportedQueueKey("session-a", "worker-1")
	k2 := task.ExportedQueueKey("session-b", "worker-1")
	if k1 != k2 {
		t.Errorf("expected same key for different sessions, got %q and %q", k1, k2)
	}
	if k1 != "worker-1" {
		t.Errorf("expected key to equal workerID, got %q", k1)
	}
}

// --- Helper managers ---

type blockingExecManager struct {
	blocker   <-chan struct{}
	started   int64
	completed int64
}

func (m *blockingExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.started, 1)
	<-m.blocker
	atomic.AddInt64(&m.completed, 1)
	return model.WorkerExecution{ID: "exec-x"}, nil
}

func (m *blockingExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

type alwaysFailExecManager struct {
	called int64
}

func (m *alwaysFailExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.called, 1)
	return model.WorkerExecution{}, fmt.Errorf("exec: \"claude\": executable file not found in $PATH")
}

func (m *alwaysFailExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

type fallbackExecManager struct {
	freshResult model.WorkerExecution
	freshCount  int64
}

func (m *fallbackExecManager) ExecuteWorker(_ context.Context, _, _, _ string, resume bool) (model.WorkerExecution, error) {
	if resume {
		return model.WorkerExecution{}, fmt.Errorf("session broken")
	}
	atomic.AddInt64(&m.freshCount, 1)
	return m.freshResult, nil
}

func (m *fallbackExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

func TestTaskDispatcher_ExecuteError_CallsFailTask(t *testing.T) {
	mgr := &alwaysFailExecManager{}
	eq := &mockExecutionQuerier{}
	fn := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task.DispatchTask{
		TaskID:      "task-launch-fail",
		WorkerID:    "w1",
		SessionKey:  "s1",
		Instruction: "do something",
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: "s1"},
		TaskType:    "countdown",
		MessageID:   "msg-1",
	}
	in <- task

	// Wait for FailTask to be called
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		n := len(ts.failedTasks)
		ts.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts.mu.Lock()
	if len(ts.failedTasks) != 1 || ts.failedTasks[0] != "task-launch-fail" {
		t.Errorf("expected FailTask called with task-launch-fail, got %v", ts.failedTasks)
	}
	ts.mu.Unlock()

	// Verify failure notification was sent.
	if !fn.waitForCall(2 * time.Second) {
		t.Fatal("expected NotifyTaskFailure to be called")
	}
	fn.mu.Lock()
	defer fn.mu.Unlock()
	if fn.calls[0].messageID != "msg-1" {
		t.Errorf("expected messageID=msg-1, got %s", fn.calls[0].messageID)
	}
}

func TestTaskDispatcher_ClearSession_OnlyRemovesMatchingSession(t *testing.T) {
	ss := newMockSessionStore()
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}

	in := make(chan task.DispatchTask, 8)
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// t1 from s1 starts executing (blocks)
	t1 := immediateTask("s1", "w1", "s1-first")
	t1.TaskID = "t1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // wait for t1 to start blocking

	// t2 from s1 queued as pending
	t2 := immediateTask("s1", "w1", "s1-second")
	t2.TaskID = "t2"
	in <- t2

	// t3 from s2 (different session, same worker) queued as pending
	t3 := immediateTask("s2", "w1", "s2-task")
	t3.TaskID = "t3"
	in <- t3

	time.Sleep(30 * time.Millisecond) // let pending tasks register

	// Clear session s1 — should remove t2 but NOT t3
	d.ClearSession("s1")
	time.Sleep(50 * time.Millisecond)

	// Unblock t1
	close(blocker)

	// Wait for t3 to execute (s2's task should still run)
	deadline2 := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline2) {
		if atomic.LoadInt64(&mgr.completed) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.completed) < 2 {
		t.Fatalf("expected 2 executions (t1 + t3), got %d", atomic.LoadInt64(&mgr.completed))
	}

	// t2 from s1 must NOT have executed
	if atomic.LoadInt64(&mgr.started) > 2 {
		t.Errorf("expected at most 2 executions started (t2 should be cleared), got %d", atomic.LoadInt64(&mgr.started))
	}

	// Session contexts for s1 must have been cleared
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if len(ss.cleared) == 0 || ss.cleared[0] != "s1" {
		t.Errorf("expected ClearSessionContexts called with s1, got %v", ss.cleared)
	}
}

// cancelTrackingExecManager blocks forever on ExecuteWorker (context-aware),
// and tracks CancelExecution calls.
type cancelTrackingExecManager struct {
	cancelCount *int64
}

func (m *cancelTrackingExecManager) ExecuteWorker(ctx context.Context, _, _, _ string, _ bool) (model.WorkerExecution, error) {
	<-ctx.Done()
	return model.WorkerExecution{ID: "exec-tracked"}, nil
}

func (m *cancelTrackingExecManager) CancelExecution(_ context.Context, _ string) error {
	atomic.AddInt64(m.cancelCount, 1)
	return nil
}

func TestTaskDispatcher_CancelTask_RemovesPendingTask(t *testing.T) {
	// A pending (not yet executing) task should be removed from the queue.
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{Status: model.ExecStatusCompleted}}

	in := make(chan task.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task.New(mgr, ts, newMockSessionStore(), eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// t1 blocks the worker queue
	t1 := immediateTask("s1", "w1", "first")
	t1.TaskID = "task-1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // t1 now executing

	// t2 is pending in queue
	t2 := immediateTask("s1", "w1", "second")
	t2.TaskID = "task-2"
	in <- t2
	time.Sleep(20 * time.Millisecond)

	// Cancel t2 while it's pending
	if err := d.CancelTask(context.Background(), "task-2"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Unblock t1
	close(blocker)
	time.Sleep(100 * time.Millisecond)

	// t2 should NOT have executed
	if atomic.LoadInt64(&mgr.completed) > 1 {
		t.Errorf("task-2 should not have executed after cancel, completed=%d", atomic.LoadInt64(&mgr.completed))
	}
}

func TestTaskDispatcher_CancelTask_InterruptsExecutingTask(t *testing.T) {
	var cancelCalled int64
	mgr := &cancelTrackingExecManager{cancelCount: &cancelCalled}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{Status: model.ExecStatusCompleted}}

	in := make(chan task.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task.New(mgr, ts, newMockSessionStore(), eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "long task")
	t1.TaskID = "task-exec-1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // executing

	if err := d.CancelTask(context.Background(), "task-exec-1"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&cancelCalled) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&cancelCalled) == 0 {
		t.Error("expected CancelExecution to be called on the manager")
	}
}

func TestDispatcher_CompleteTask_OnSuccessfulExit(t *testing.T) {
	mgr := &mockExecManager{execResult: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusRunning}}
	ts := &mockTaskStore{}
	execStore := &mockExecutionQuerier{result: model.WorkerExecution{
		ID:     "exec-1",
		Status: model.ExecStatusCompleted,
	}}
	ss := newMockSessionStore()

	ch := make(chan task.DispatchTask, 1)
	d := task.New(mgr, ts, ss, execStore, ch)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go d.Run(ctx)

	ch <- task.DispatchTask{
		TaskID:   "task-1",
		WorkerID: "worker-1",
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		done := len(ts.completedTasks) > 0
		ts.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.completedTasks) != 1 || ts.completedTasks[0] != "task-1" {
		t.Errorf("want completedTasks=[task-1], got %v", ts.completedTasks)
	}
	if len(ts.failedTasks) != 0 {
		t.Errorf("want no failedTasks, got %v", ts.failedTasks)
	}
}

func TestDispatcher_BuildInstruction_MessageIDWithoutTaskID(t *testing.T) {
	dispatchCh := make(chan task.DispatchTask, 8)
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{
			ID:        "exec-1",
			SessionID: "sess-1",
			Status:    model.ExecStatusCompleted,
		},
	}
	querier := &mockExecutionQuerier{result: model.WorkerExecution{
		ID:     "exec-1",
		Status: model.ExecStatusCompleted,
	}}
	taskStore := &mockTaskStore{}
	sessionStore := newMockSessionStore()

	d := task.New(mgr, taskStore, sessionStore, querier, dispatchCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	dispatchCh <- task.DispatchTask{
		TaskID:      "",
		MessageID:   "msg-abc",
		WorkerID:    "w1",
		SessionKey:  "sk1",
		Instruction: "do something",
		TaskType:    model.TaskTypeImmediate,
	}

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("expected worker to be called")
	}

	mgr.mu.Lock()
	instructions := mgr.executedInstructions
	mgr.mu.Unlock()

	if len(instructions) == 0 {
		t.Fatal("expected worker to be called")
	}
	instr := instructions[0]
	wantMeta := `<task_meta>{"message_id":"msg-abc"}</task_meta>`
	if !strings.Contains(instr, wantMeta) {
		t.Errorf("expected message_id in task_meta, got:\n%s", instr)
	}
	if !strings.Contains(instr, "<task_content>") {
		t.Errorf("expected task_content tag in instruction, got:\n%s", instr)
	}
	if !strings.Contains(instr, "</task_content>") {
		t.Errorf("instruction missing closing task_content tag, got: %q", instr)
	}
	if strings.Contains(instr, "task_id") {
		t.Errorf("expected no task_id in instruction when TaskID is empty, got:\n%s", instr)
	}
}

func TestTaskDispatcher_ExecStatusFailed_CallsFailTask(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-fail", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{
		result: model.WorkerExecution{ID: "exec-fail", Status: model.ExecStatusFailed, Result: "API Error: blocked"},
	}
	fn := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task.DispatchTask{
		TaskID:      "task-fail-1",
		WorkerID:    "w1",
		SessionKey:  "s1",
		Instruction: "do something",
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: "s1"},
		TaskType:    "immediate",
		MessageID:   "msg-1",
	}
	in <- task

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}
	// Wait for FailTask to be called
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		n := len(ts.failedTasks)
		ts.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts.mu.Lock()
	if len(ts.failedTasks) != 1 || ts.failedTasks[0] != "task-fail-1" {
		t.Errorf("expected FailTask called with task-fail-1, got %v", ts.failedTasks)
	}
	ts.mu.Unlock()

	// Verify failure notification was sent with execution result.
	if !fn.waitForCall(2 * time.Second) {
		t.Fatal("expected NotifyTaskFailure to be called")
	}
	fn.mu.Lock()
	defer fn.mu.Unlock()
	if fn.calls[0].messageID != "msg-1" {
		t.Errorf("expected messageID=msg-1, got %s", fn.calls[0].messageID)
	}
	if fn.calls[0].info.Reason != "API Error: blocked" {
		t.Errorf("expected reason='API Error: blocked', got %s", fn.calls[0].info.Reason)
	}
}

func TestDispatcher_BuildInstruction_NoMetadata(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- task.DispatchTask{
		TaskID:      "",
		MessageID:   "",
		WorkerID:    "w1",
		SessionKey:  "s1",
		Instruction: "raw instruction",
		TaskType:    model.TaskTypeImmediate,
	}

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("expected worker to be called")
	}

	mgr.mu.Lock()
	instructions := mgr.executedInstructions
	mgr.mu.Unlock()

	if len(instructions) == 0 {
		t.Fatal("expected worker to be called")
	}
	got := instructions[0]
	// New sessions get the skill hint prefix; the raw instruction follows after the newline.
	if !strings.Contains(got, "raw instruction") {
		t.Errorf("expected instruction to contain original text, got: %q", got)
	}
}

func TestTaskDispatcher_NewSession_HasSkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	// No prior session context — new session
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	tsk := immediateTask("sk-1", "worker-1", "do the thing")
	in <- tsk

	if !waitForExecCount(mgr, 1, 3*time.Second) {
		t.Fatal("timeout waiting for execution")
	}
	mgr.mu.Lock()
	instruction := mgr.executedInstructions[0]
	mgr.mu.Unlock()
	if !strings.HasPrefix(instruction, ai.SkillHintPrefix(ai.RoleWorker)) {
		t.Errorf("new session must start with skill hint\ngot: %q", instruction)
	}
}

func TestTaskDispatcher_ResumeSession_NoSkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	ss := newMockSessionStore()
	// Pre-populate session context so this is a resume.
	// Engine name must match enginecfg.Set value so
	// GetSessionContextForEngine returns the stored session ID.
	_ = ss.UpsertSessionContext(context.Background(), "sk-1", "worker-1", "existing-sess", "testengine")
	enginecfg.Set("testengine")
	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	tsk := immediateTask("sk-1", "worker-1", "do the thing")
	in <- tsk

	if !waitForExecCount(mgr, 1, 3*time.Second) {
		t.Fatal("timeout waiting for execution")
	}
	mgr.mu.Lock()
	instruction := mgr.executedInstructions[0]
	mgr.mu.Unlock()
	if strings.HasPrefix(instruction, ai.SkillHintPrefix(ai.RoleWorker)) {
		t.Errorf("resume session must NOT have skill hint\ngot: %q", instruction)
	}
}

func TestTaskDispatcher_NewSession_InjectsWorkerPersona(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	lookup := &mockWorkerLookup{
		worker: model.Worker{
			ID:          "w1",
			Name:        "毛毛",
			Description: "负责 openbee 开发",
			Memory:      "记住老板的偏好",
		},
	}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(),
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	if !strings.HasPrefix(instr, ai.SkillHintPrefix(ai.RoleWorker)) {
		t.Errorf("instruction missing skill hint prefix, got: %q", instr)
	}
	if !strings.Contains(instr, "<worker_persona>") {
		t.Errorf("instruction missing <worker_persona> tag, got: %q", instr)
	}
	if !strings.Contains(instr, "Name: 毛毛") {
		t.Errorf("instruction missing worker name, got: %q", instr)
	}
	if !strings.Contains(instr, "Description: 负责 openbee 开发") {
		t.Errorf("instruction missing worker description, got: %q", instr)
	}
	if !strings.Contains(instr, "记住老板的偏好") {
		t.Errorf("instruction missing worker memory, got: %q", instr)
	}
	if !strings.Contains(instr, "</worker_persona>") {
		t.Errorf("instruction missing </worker_persona> tag, got: %q", instr)
	}
}

func TestTaskDispatcher_NewSession_NilLookup_OnlySkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	if !strings.HasPrefix(instr, ai.SkillHintPrefix(ai.RoleWorker)) {
		t.Errorf("instruction missing skill hint prefix, got: %q", instr)
	}
	if strings.Contains(instr, "<worker_persona>") {
		t.Errorf("instruction should not contain <worker_persona> when lookup is nil, got: %q", instr)
	}
}

func TestTaskDispatcher_NewSession_LookupError_FailsTask(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	lookup := &mockWorkerLookup{err: fmt.Errorf("worker not found")}
	notifier := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(),
		task.WithWorkerLookup(lookup),
		task.WithFailureNotifier(notifier),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "do the thing")
	t1.TaskID = "task-fail"
	t1.MessageID = "msg-fail"
	in <- t1

	if !notifier.waitForCall(2 * time.Second) {
		t.Fatal("failure notifier was not called within timeout")
	}

	ts.mu.Lock()
	failed := ts.failedTasks
	ts.mu.Unlock()
	if len(failed) == 0 || failed[0] != "task-fail" {
		t.Errorf("expected task-fail to be failed, got %v", failed)
	}
	mgr.mu.Lock()
	execCount := len(mgr.executedInstructions)
	mgr.mu.Unlock()
	if execCount != 0 {
		t.Errorf("ExecuteWorker should not be called on lookup error, got %d calls", execCount)
	}
}

func TestTaskDispatcher_FreshSession_PreflightUpsertBeforeExecute(t *testing.T) {
	mgr := &orderedMockManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "new-session"},
	}
	baseSS := newMockSessionStore()
	ss := &orderedMockSessionStore{mockSessionStore: baseSS, outer: mgr}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "first message")

	if !waitFor(func() int { return int(mgr.executed.Load()) }, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	order := append([]string{}, mgr.callOrder...)
	resume := mgr.receivedResume
	sessID := mgr.receivedSessID
	mgr.mu.Unlock()

	if len(order) < 2 {
		t.Fatalf("expected at least 2 calls (upsert + execute), got %v", order)
	}
	if order[0] != "upsert" || order[1] != "execute" {
		t.Errorf("expected upsert before execute, got order %v", order)
	}
	if resume {
		t.Error("expected resume=false for fresh session")
	}
	if sessID == "" {
		t.Error("expected non-empty sessionID passed to ExecuteWorker")
	}
}

func TestTaskDispatcher_ResumeSession_PreflightUpsertBeforeExecute(t *testing.T) {
	mgr := &orderedMockManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
	}
	baseSS := newMockSessionStore()
	_ = baseSS.UpsertSessionContext(context.Background(), "s1", "w1", "prior-session-id", "claude")
	ss := &orderedMockSessionStore{mockSessionStore: baseSS, outer: mgr}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

	enginecfg.Set("claude")
	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "follow-up")

	if !waitFor(func() int { return int(mgr.executed.Load()) }, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	order := append([]string{}, mgr.callOrder...)
	resume := mgr.receivedResume
	sessID := mgr.receivedSessID
	mgr.mu.Unlock()

	if len(order) < 2 {
		t.Fatalf("expected at least 2 calls (upsert + execute), got %v", order)
	}
	if order[0] != "upsert" || order[1] != "execute" {
		t.Errorf("expected upsert before execute, got order %v", order)
	}
	if !resume {
		t.Error("expected resume=true for existing session")
	}
	if sessID != "prior-session-id" {
		t.Errorf("expected prior-session-id, got %q", sessID)
	}
}

func TestTaskDispatcher_WorkerEngine_UsedInSessionContext(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-pi-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	ss := newMockSessionStore()
	lookup := &mockWorkerLookup{
		worker: model.Worker{ID: "w1", Engine: "pi"},
	}
	// System default is "kimi", but the worker is configured with "pi".
	enginecfg.Set("kimi")
	d, in, _ := newTaskDispatcher(mgr, eq, ss,
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("sk-1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("timeout waiting for execution")
	}

	// Session context must be stored under the worker's engine ("pi"), not the system default ("kimi").
	if got := ss.sessionID("sk-1", "w1", "pi"); got == "" {
		t.Error("expected session context stored under engine 'pi', got nothing")
	}
	if got := ss.sessionID("sk-1", "w1", "kimi"); got != "" {
		t.Errorf("session context must not be stored under system-default engine 'kimi', got %q", got)
	}
}
