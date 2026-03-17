package task_dispatcher_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robobee/core/internal/task_dispatcher"
	"github.com/robobee/core/internal/model"
	"github.com/robobee/core/internal/platform"
)

// --- Mocks ---

type mockExecManager struct {
	mu                   sync.Mutex
	execResult           model.WorkerExecution
	resumedWithSessionID string
	executedInstructions []string
}

func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string) (model.WorkerExecution, error) {
	m.mu.Lock()
	if sessionID != "" {
		m.resumedWithSessionID = sessionID
	}
	m.executedInstructions = append(m.executedInstructions, instruction)
	m.mu.Unlock()
	return m.execResult, nil
}

type mockExecutionQuerier struct {
	result model.WorkerExecution
}

func (m *mockExecutionQuerier) GetByID(_ string) (model.WorkerExecution, error) {
	return m.result, nil
}

type mockTaskStore struct {
	mu          sync.Mutex
	failedTasks []string
}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (s *mockTaskStore) FailTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedTasks = append(s.failedTasks, taskID)
	return nil
}

type mockSessionStore struct {
	mu      sync.Mutex
	data    map[string]string
	cleared []string
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{data: make(map[string]string)}
}

func (s *mockSessionStore) GetSessionContext(_ context.Context, sessionKey, agentID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[sessionKey+"|"+agentID], nil
}
func (s *mockSessionStore) UpsertSessionContext(_ context.Context, sessionKey, agentID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionKey+"|"+agentID] = sessionID
	return nil
}
func (s *mockSessionStore) ClearSessionContexts(_ context.Context, sessionKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleared = append(s.cleared, sessionKey)
	return nil
}

type mockFailureNotifier struct {
	mu        sync.Mutex
	calls     []failureCall
}

type failureCall struct {
	messageID string
	reason    string
}

func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID, reason string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, failureCall{messageID: messageID, reason: reason})
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

func newTaskDispatcher(mgr task_dispatcher.ExecutionManager, eq task_dispatcher.ExecutionQuerier, ss task_dispatcher.SessionStore, opts ...task_dispatcher.Option) (*task_dispatcher.TaskDispatcher, chan task_dispatcher.DispatchTask, *mockTaskStore) {
	in := make(chan task_dispatcher.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task_dispatcher.New(mgr, ts, ss, eq, in, opts...)
	return d, in, ts
}

func immediateTask(sessionKey, workerID, instruction string) task_dispatcher.DispatchTask {
	return task_dispatcher.DispatchTask{
		TaskID:      "task-1",
		WorkerID:    workerID,
		SessionKey:  sessionKey,
		Instruction: instruction,
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: sessionKey},
		TaskType:    "immediate",
		MessageID:   "msg-1",
	}
}

// waitForExecCount waits until mgr.executedInstructions reaches n or timeout.
func waitForExecCount(mgr *mockExecManager, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mgr.mu.Lock()
		count := len(mgr.executedInstructions)
		mgr.mu.Unlock()
		if count >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
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

	task := task_dispatcher.DispatchTask{
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

	if !strings.Contains(instr, "task_id=task-abc") {
		t.Errorf("instruction missing task_id injection, got: %q", instr)
	}
	if !strings.Contains(instr, "message_id=msg-xyz") {
		t.Errorf("instruction missing message_id injection, got: %q", instr)
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
	in := make(chan task_dispatcher.DispatchTask, 4)
	d := task_dispatcher.New(mgr, &mockTaskStore{}, ss, eq, in)

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
	ss.data["s1|w1"] = "prior-session-id"

	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted, Result: "resumed!"}}
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
	ss.data["s1|w1"] = "broken-session-id"

	mgr := &fallbackExecManager{
		freshResult: model.WorkerExecution{ID: "exec-fresh", SessionID: "new-session"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-fresh", Status: model.ExecStatusCompleted, Result: "fallback-ok"}}

	in := make(chan task_dispatcher.DispatchTask, 4)
	d := task_dispatcher.New(mgr, &mockTaskStore{}, ss, eq, in)

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

	// Stale session should be cleared
	deadline = time.Now().Add(500 * time.Millisecond)
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
	if len(ss.cleared) == 0 {
		t.Error("expected ClearSessionContexts called after resume failure")
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

// --- Helper managers ---

type blockingExecManager struct {
	blocker   <-chan struct{}
	started   int64
	completed int64
}

func (m *blockingExecManager) ExecuteWorker(_ context.Context, _, _, _ string) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.started, 1)
	<-m.blocker
	atomic.AddInt64(&m.completed, 1)
	return model.WorkerExecution{ID: "exec-x"}, nil
}

type alwaysFailExecManager struct {
	called int64
}

func (m *alwaysFailExecManager) ExecuteWorker(_ context.Context, _, _, _ string) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.called, 1)
	return model.WorkerExecution{}, fmt.Errorf("exec: \"claude\": executable file not found in $PATH")
}

type fallbackExecManager struct {
	freshResult model.WorkerExecution
	freshCount  int64
}

func (m *fallbackExecManager) ExecuteWorker(_ context.Context, _, _, sessionID string) (model.WorkerExecution, error) {
	if sessionID != "" {
		return model.WorkerExecution{}, fmt.Errorf("session broken")
	}
	atomic.AddInt64(&m.freshCount, 1)
	return m.freshResult, nil
}

func TestTaskDispatcher_ExecuteError_CallsFailTask(t *testing.T) {
	mgr := &alwaysFailExecManager{}
	eq := &mockExecutionQuerier{}
	fn := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(), task_dispatcher.WithFailureNotifier(fn))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task_dispatcher.DispatchTask{
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

func TestTaskDispatcher_ExecStatusFailed_CallsFailTask(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-fail", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{
		result: model.WorkerExecution{ID: "exec-fail", Status: model.ExecStatusFailed, Result: "API Error: blocked"},
	}
	fn := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(), task_dispatcher.WithFailureNotifier(fn))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task_dispatcher.DispatchTask{
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
	if fn.calls[0].reason != "API Error: blocked" {
		t.Errorf("expected reason='API Error: blocked', got %s", fn.calls[0].reason)
	}
}
