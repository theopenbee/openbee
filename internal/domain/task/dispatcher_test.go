package task_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/domain/worker"
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

func (m *mockExecManager) ExecuteWorker(_ context.Context, req worker.ExecuteRequest) (model.WorkerExecution, error) {
	m.mu.Lock()
	if req.Resume {
		m.resumedWithSessionID = req.SessionID
	}
	m.executedInstructions = append(m.executedInstructions, req.TriggerInput)
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

// cancelGateQuerier blocks GetByID until its gate channel is closed, then returns a DB error.
// This lets tests trigger the cancel-during-poll-error path in waitForResult.
type cancelGateQuerier struct {
	gate chan struct{}
}

func (q *cancelGateQuerier) GetByID(_ string) (model.WorkerExecution, error) {
	<-q.gate
	return model.WorkerExecution{}, errors.New("db poll error")
}

// quickCancelExecManager returns immediately from ExecuteWorker and tracks CancelExecution calls.
type quickCancelExecManager struct {
	execCalled  chan struct{}
	cancelCount *int64
}

func (m *quickCancelExecManager) ExecuteWorker(_ context.Context, _ worker.ExecuteRequest) (model.WorkerExecution, error) {
	select {
	case m.execCalled <- struct{}{}:
	default:
	}
	return model.WorkerExecution{}, nil
}

func (m *quickCancelExecManager) CancelExecution(_ context.Context, _ string) error {
	atomic.AddInt64(m.cancelCount, 1)
	return nil
}

type mockTaskStore struct {
	mu             sync.Mutex
	failedTasks    []string
	completedTasks []string
}

func (s *mockTaskStore) UpdateStatus(_ context.Context, _, _ string) error { return nil }
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
func (s *mockSessionStore) DeleteSessionContextForEngine(_ context.Context, sessionKey, agentID, engine string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := newMockSessionRef(sessionKey, agentID, engine)
	_, existed := s.data[ref]
	delete(s.data, ref)
	s.deleted = append(s.deleted, ref)
	return existed, nil
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
	mu          sync.Mutex
	calls       []failureCall
	cancelCalls []cancelCall
}

type failureCall struct {
	messageID string
	info      model.FailureInfo
}

type cancelCall struct {
	messageID  string
	workerName string
}

func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, failureCall{messageID: messageID, info: info})
	return nil
}

func (n *mockFailureNotifier) NotifyTaskCancelled(_ context.Context, messageID string, workerName string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cancelCalls = append(n.cancelCalls, cancelCall{messageID: messageID, workerName: workerName})
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

func (n *mockFailureNotifier) waitForCancelCall(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		count := len(n.cancelCalls)
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

func (m *orderedMockManager) ExecuteWorker(_ context.Context, req worker.ExecuteRequest) (model.WorkerExecution, error) {
	m.mu.Lock()
	m.callOrder = append(m.callOrder, "execute")
	m.receivedResume = req.Resume
	m.receivedSessID = req.SessionID
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
	return newTaskDispatcherWithEngine(mgr, eq, ss, "", opts...)
}

func newTaskDispatcherWithEngine(mgr task.ExecutionManager, eq task.ExecutionQuerier, ss task.SessionStore, engine string, opts ...task.Option) (*task.TaskDispatcher, chan task.DispatchTask, *mockTaskStore) {
	in := make(chan task.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task.New(mgr, ts, ss, eq, in, enginecfg.NewStore(engine), opts...)
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

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorker was not called within timeout")
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

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorker was not called within timeout")

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	wantMeta := `<task_meta>{"message_id":"msg-xyz","task_id":"task-abc"}</task_meta>`
	assert.Contains(t, instr, wantMeta)
	assert.Contains(t, instr, "<task_content>")
	assert.Contains(t, instr, "</task_content>")
	assert.Contains(t, instr, "do the thing")
}

func TestTaskDispatcher_ClearSession_ClearsQueueAndSessionContexts(t *testing.T) {
	ss := newMockSessionStore()
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}

	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted, Result: "ok"}}
	in := make(chan task.DispatchTask, 4)
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in, enginecfg.NewStore(""))

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
	assert.Equal(t, []string{"s1"}, cleared)

	// Second task should NOT have executed (queue was cleared)
	assert.LessOrEqual(t, atomic.LoadInt64(&mgr.completed), int64(1), "second should be cleared from queue")
}

func TestTaskDispatcher_ImmediateTask_SessionChoice(t *testing.T) {
	cases := []struct {
		name        string
		existing    bool
		instruction string
		execResult  model.WorkerExecution
		execOutcome string
		wantResume  string
	}{
		{
			name:        "ResumesWhenSessionExists",
			existing:    true,
			instruction: "follow-up",
			execResult:  model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
			execOutcome: "resumed!",
			wantResume:  "prior-session-id",
		},
		{
			name:        "FreshWhenNoSession",
			instruction: "first message",
			execResult:  model.WorkerExecution{ID: "exec-1", SessionID: "new-session"},
			execOutcome: "fresh!",
			wantResume:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ss := newMockSessionStore()
			engine := ""
			if tc.existing {
				engine = "claude"
				_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "prior-session-id", "claude")
			}
			mgr := &mockExecManager{execResult: tc.execResult}
			eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted, Result: tc.execOutcome}}
			d, in, _ := newTaskDispatcherWithEngine(mgr, eq, ss, engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			in <- immediateTask("s1", "w1", tc.instruction)

			require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorkerWithSession was not called within timeout")

			mgr.mu.Lock()
			resumed := mgr.resumedWithSessionID
			mgr.mu.Unlock()

			assert.Equal(t, tc.wantResume, resumed)
		})
	}
}

func TestTaskDispatcher_ImmediateTask_EngineSwitch_PreservesPriorSession(t *testing.T) {
	ss := newMockSessionStore()
	_ = ss.UpsertSessionContext(context.Background(), "s1", "w1", "claude-session-id", "claude")

	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "codex-session-id"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", SessionID: "codex-session-id", Status: model.ExecStatusCompleted, Result: "fresh!"}}
	d, in, _ := newTaskDispatcherWithEngine(mgr, eq, ss, "codex")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "switch engine")

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorker was not called within timeout")

	mgr.mu.Lock()
	resumed := mgr.resumedWithSessionID
	mgr.mu.Unlock()
	assert.Empty(t, resumed, "expected fresh start on engine switch")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ss.sessionID("s1", "w1", "codex") == "codex-session-id" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, "claude-session-id", ss.sessionID("s1", "w1", "claude"), "expected claude session preserved")
	assert.Equal(t, "codex-session-id", ss.sessionID("s1", "w1", "codex"))
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
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in, enginecfg.NewStore("codex"))

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
	require.GreaterOrEqual(t, atomic.LoadInt64(&mgr.freshCount), int64(1), "fallback ExecuteWorker was never called")

	// Stale codex session should be deleted before the fresh run is started.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ss.deleteCount() > 0 && ss.sessionID("s1", "w1", "codex") == "new-session" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	deletedRef, ok := ss.deletedRef(0)
	assert.True(t, ok, "expected a deleted session ref")
	assert.Equal(t, newMockSessionRef("s1", "w1", "codex"), deletedRef)
	assert.Empty(t, ss.cleared, "did not expect full session clear on resume failure")
	assert.Equal(t, "claude-session-id", ss.sessionID("s1", "w1", "claude"), "expected claude session preserved")
	assert.Equal(t, "new-session", ss.sessionID("s1", "w1", "codex"), "expected codex session refreshed after fallback")
}

func TestTaskDispatcher_Serialized(t *testing.T) {
	cases := []struct {
		name          string
		secondSession string
	}{
		{name: "TwoTasks_SameSession", secondSession: "s1"},
		{name: "CrossSession_SameWorker", secondSession: "s2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocker := make(chan struct{})
			mgr := &blockingExecManager{blocker: blocker}
			eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}
			d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			t1 := immediateTask("s1", "w1", "first")
			t1.TaskID = "task-1"
			t2 := immediateTask(tc.secondSession, "w1", "second")
			t2.TaskID = "task-2"

			in <- t1
			in <- t2

			// Wait for first task to start blocking
			time.Sleep(50 * time.Millisecond)
			require.Equal(t, int64(1), atomic.LoadInt64(&mgr.started), "expected 1 execution started (second should be queued)")

			// Unblock first execution
			close(blocker)

			// Both should eventually complete
			waitFor(func() int { return int(atomic.LoadInt64(&mgr.completed)) }, 2, 2*time.Second)
			assert.GreaterOrEqual(t, atomic.LoadInt64(&mgr.completed), int64(2), "expected both tasks to complete")
		})
	}
}

// --- Helper managers ---

type blockingExecManager struct {
	blocker   <-chan struct{}
	started   int64
	completed int64
}

func (m *blockingExecManager) ExecuteWorker(_ context.Context, _ worker.ExecuteRequest) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.started, 1)
	<-m.blocker
	atomic.AddInt64(&m.completed, 1)
	return model.WorkerExecution{ID: "exec-x"}, nil
}

func (m *blockingExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

// tableExecManager returns execErr from ExecuteWorker when set; otherwise it
// succeeds with execResult. Lets table-driven tests parameterize whether a
// task fails via a launch error or via a terminal "failed" execution status.
type tableExecManager struct {
	execErr    error
	execResult model.WorkerExecution
	executed   atomic.Int64
}

func (m *tableExecManager) ExecuteWorker(_ context.Context, _ worker.ExecuteRequest) (model.WorkerExecution, error) {
	m.executed.Add(1)
	if m.execErr != nil {
		return model.WorkerExecution{}, m.execErr
	}
	return m.execResult, nil
}

func (m *tableExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

type fallbackExecManager struct {
	freshResult model.WorkerExecution
	freshCount  int64
}

func (m *fallbackExecManager) ExecuteWorker(_ context.Context, req worker.ExecuteRequest) (model.WorkerExecution, error) {
	if req.Resume {
		return model.WorkerExecution{}, fmt.Errorf("session broken")
	}
	atomic.AddInt64(&m.freshCount, 1)
	return m.freshResult, nil
}

func (m *fallbackExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

func TestTaskDispatcher_FailureCallsFailTask(t *testing.T) {
	cases := []struct {
		name       string
		execErr    error
		execStatus model.ExecutionStatus
		taskID     string
		taskType   string
		execResult model.WorkerExecution
		wantReason string
	}{
		{
			name:     "ExecuteError",
			execErr:  fmt.Errorf("exec: \"claude\": executable file not found in $PATH"),
			taskID:   "task-launch-fail",
			taskType: "countdown",
		},
		{
			name:       "ExecStatusFailed",
			execStatus: model.ExecStatusFailed,
			taskID:     "task-fail-1",
			taskType:   "immediate",
			execResult: model.WorkerExecution{ID: "exec-fail", SessionID: "sess-1"},
			wantReason: "API Error: blocked",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &tableExecManager{execErr: tc.execErr, execResult: tc.execResult}
			eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-fail", Status: tc.execStatus, Result: tc.wantReason}}
			fn := &mockFailureNotifier{}
			d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			in <- task.DispatchTask{
				TaskID:      tc.taskID,
				WorkerID:    "w1",
				SessionKey:  "s1",
				Instruction: "do something",
				ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: "s1"},
				TaskType:    tc.taskType,
				MessageID:   "msg-1",
			}

			if tc.execStatus != "" {
				require.True(t, waitFor(func() int { return int(mgr.executed.Load()) }, 1, 2*time.Second), "ExecuteWorker was not called within timeout")
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
			assert.Equal(t, []string{tc.taskID}, ts.failedTasks)
			ts.mu.Unlock()

			// Verify failure notification was sent.
			require.True(t, fn.waitForCall(2*time.Second), "expected NotifyTaskFailure to be called")
			fn.mu.Lock()
			defer fn.mu.Unlock()
			assert.Equal(t, "msg-1", fn.calls[0].messageID)
			if tc.wantReason != "" {
				assert.Equal(t, tc.wantReason, fn.calls[0].info.Reason)
			}
		})
	}
}

func TestTaskDispatcher_ClearSession_OnlyRemovesMatchingSession(t *testing.T) {
	ss := newMockSessionStore()
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}

	in := make(chan task.DispatchTask, 8)
	d := task.New(mgr, &mockTaskStore{}, ss, eq, in, enginecfg.NewStore(""))

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
	require.GreaterOrEqual(t, atomic.LoadInt64(&mgr.completed), int64(2), "expected 2 executions (t1 + t3)")

	// t2 from s1 must NOT have executed
	assert.LessOrEqual(t, atomic.LoadInt64(&mgr.started), int64(2), "expected at most 2 executions started (t2 should be cleared)")

	// Session contexts for s1 must have been cleared
	ss.mu.Lock()
	defer ss.mu.Unlock()
	assert.Equal(t, []string{"s1"}, ss.cleared)
}

// cancelTrackingExecManager blocks forever on ExecuteWorker (context-aware),
// and tracks CancelExecution calls.
type cancelTrackingExecManager struct {
	cancelCount *int64
}

func (m *cancelTrackingExecManager) ExecuteWorker(ctx context.Context, _ worker.ExecuteRequest) (model.WorkerExecution, error) {
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
	d := task.New(mgr, ts, newMockSessionStore(), eq, in, enginecfg.NewStore(""))

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
	require.NoError(t, d.CancelTask(context.Background(), "task-2"))
	time.Sleep(50 * time.Millisecond)

	// Unblock t1
	close(blocker)
	time.Sleep(100 * time.Millisecond)

	// t2 should NOT have executed
	assert.LessOrEqual(t, atomic.LoadInt64(&mgr.completed), int64(1), "task-2 should not have executed after cancel")
}

func TestTaskDispatcher_CancelTask_InterruptsExecutingTask(t *testing.T) {
	var cancelCalled int64
	mgr := &cancelTrackingExecManager{cancelCount: &cancelCalled}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{Status: model.ExecStatusCompleted}}

	in := make(chan task.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task.New(mgr, ts, newMockSessionStore(), eq, in, enginecfg.NewStore(""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "long task")
	t1.TaskID = "task-exec-1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // executing

	require.NoError(t, d.CancelTask(context.Background(), "task-exec-1"))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&cancelCalled) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotZero(t, atomic.LoadInt64(&cancelCalled), "expected CancelExecution to be called on the manager")
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
	d := task.New(mgr, ts, ss, execStore, ch, enginecfg.NewStore(""))

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
	assert.Equal(t, []string{"task-1"}, ts.completedTasks)
	assert.Empty(t, ts.failedTasks)
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

	d := task.New(mgr, taskStore, sessionStore, querier, dispatchCh, enginecfg.NewStore(""))
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

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "expected worker to be called")

	mgr.mu.Lock()
	instructions := mgr.executedInstructions
	mgr.mu.Unlock()

	require.NotEmpty(t, instructions, "expected worker to be called")
	instr := instructions[0]
	wantMeta := `<task_meta>{"message_id":"msg-abc"}</task_meta>`
	assert.Contains(t, instr, wantMeta)
	assert.Contains(t, instr, "<task_content>")
	assert.Contains(t, instr, "</task_content>")
	assert.NotContains(t, instr, "task_id", "expected no task_id in instruction when TaskID is empty")
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

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "expected worker to be called")

	mgr.mu.Lock()
	instructions := mgr.executedInstructions
	mgr.mu.Unlock()

	require.NotEmpty(t, instructions, "expected worker to be called")
	got := instructions[0]
	// New sessions get the session prefix; the raw instruction follows after the newline.
	assert.Contains(t, got, "raw instruction")
}

func TestTaskDispatcher_SessionPrefix(t *testing.T) {
	cases := []struct {
		name       string
		existing   bool
		wantPrefix bool
	}{
		{name: "NewSession_HasSessionPrefix", existing: false, wantPrefix: true},
		{name: "ResumeSession_NoSessionPrefix", existing: true, wantPrefix: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &mockExecManager{execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1"}}
			eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
			ss := newMockSessionStore()
			engine := ""
			if tc.existing {
				// Engine name must match the dispatcher's engineCfg so
				// GetSessionContextForEngine returns the stored session ID.
				engine = "testengine"
				_ = ss.UpsertSessionContext(context.Background(), "sk-1", "worker-1", "existing-sess", "testengine")
			}
			d, in, _ := newTaskDispatcherWithEngine(mgr, eq, ss, engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			tsk := immediateTask("sk-1", "worker-1", "do the thing")
			in <- tsk

			require.True(t, waitForExecCount(mgr, 1, 3*time.Second), "timeout waiting for execution")
			mgr.mu.Lock()
			instruction := mgr.executedInstructions[0]
			mgr.mu.Unlock()

			if tc.wantPrefix {
				assert.Contains(t, instruction, "## Step 1: Initialize your role")
			} else {
				assert.NotContains(t, instruction, "## Step 1: Initialize your role")
			}
		})
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
			Constraints: "记住老板的偏好",
		},
	}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(),
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorker was not called within timeout")

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	assert.Contains(t, instr, "## Step 1: Initialize your role")
	step2Idx := strings.Index(instr, "## Step 2:")
	personaIdx := strings.Index(instr, "<worker_persona>")
	if assert.GreaterOrEqual(t, step2Idx, 0, "instruction missing Step 2 header") {
		assert.True(t, personaIdx >= 0 && personaIdx < step2Idx, "persona block must appear before Step 2")
	}
	assert.Contains(t, instr, "<worker_persona>")
	assert.Contains(t, instr, "Name: 毛毛")
	assert.Contains(t, instr, "Description: 负责 openbee 开发")
	assert.Contains(t, instr, "记住老板的偏好")
	assert.Contains(t, instr, "</worker_persona>")
}

func TestTaskDispatcher_NewSession_NilLookup_NoPersona(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "ExecuteWorker was not called within timeout")

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	assert.Contains(t, instr, "## Step 1: Initialize your role")
	assert.NotContains(t, instr, "<worker_persona>", "instruction should not contain <worker_persona> when lookup is nil")
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

	require.True(t, notifier.waitForCall(2*time.Second), "failure notifier was not called within timeout")

	ts.mu.Lock()
	failed := ts.failedTasks
	ts.mu.Unlock()
	assert.Equal(t, []string{"task-fail"}, failed)

	mgr.mu.Lock()
	execCount := len(mgr.executedInstructions)
	mgr.mu.Unlock()
	assert.Zero(t, execCount, "ExecuteWorker should not be called on lookup error")
}

func TestTaskDispatcher_PreflightUpsertBeforeExecute(t *testing.T) {
	cases := []struct {
		name        string
		existing    bool
		instruction string
		execResult  model.WorkerExecution
		wantResume  bool
		wantSessID  string
	}{
		{
			name:        "FreshSession",
			instruction: "first message",
			execResult:  model.WorkerExecution{ID: "exec-1", SessionID: "new-session"},
			wantResume:  false,
		},
		{
			name:        "ResumeSession",
			existing:    true,
			instruction: "follow-up",
			execResult:  model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
			wantResume:  true,
			wantSessID:  "prior-session-id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseSS := newMockSessionStore()
			engine := ""
			if tc.existing {
				engine = "claude"
				_ = baseSS.UpsertSessionContext(context.Background(), "s1", "w1", "prior-session-id", "claude")
			}
			mgr := &orderedMockManager{execResult: tc.execResult}
			ss := &orderedMockSessionStore{mockSessionStore: baseSS, outer: mgr}
			eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

			d, in, _ := newTaskDispatcherWithEngine(mgr, eq, ss, engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			in <- immediateTask("s1", "w1", tc.instruction)

			require.True(t, waitFor(func() int { return int(mgr.executed.Load()) }, 1, 2*time.Second), "ExecuteWorker was not called within timeout")

			mgr.mu.Lock()
			order := append([]string{}, mgr.callOrder...)
			resume := mgr.receivedResume
			sessID := mgr.receivedSessID
			mgr.mu.Unlock()

			require.GreaterOrEqual(t, len(order), 2, "expected at least 2 calls (upsert + execute)")
			assert.Equal(t, "upsert", order[0])
			assert.Equal(t, "execute", order[1])
			assert.Equal(t, tc.wantResume, resume)
			if tc.wantSessID != "" {
				assert.Equal(t, tc.wantSessID, sessID)
			} else {
				assert.NotEmpty(t, sessID, "expected non-empty sessionID passed to ExecuteWorker")
			}
		})
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
	// System default is "codex", but the worker is configured with "pi".
	d, in, _ := newTaskDispatcherWithEngine(mgr, eq, ss, "codex",
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("sk-1", "w1", "do the thing")

	require.True(t, waitForExecCount(mgr, 1, 2*time.Second), "timeout waiting for execution")

	// Session context must be stored under the worker's engine ("pi"), not the system default ("codex").
	assert.NotEmpty(t, ss.sessionID("sk-1", "w1", "pi"), "expected session context stored under engine 'pi'")
	assert.Empty(t, ss.sessionID("sk-1", "w1", "codex"), "session context must not be stored under system-default engine 'codex'")
}

func TestTaskDispatcher_CancelNotifies(t *testing.T) {
	cases := []struct {
		name           string
		setup          func() (task.ExecutionManager, task.ExecutionQuerier)
		instruction    string
		taskID         string
		messageID      string
		cancelAt       func(t *testing.T, mgr task.ExecutionManager)
		wantWorkerName string
	}{
		{
			name: "WhileWaitingForResult",
			setup: func() (task.ExecutionManager, task.ExecutionQuerier) {
				mgr := &mockExecManager{execResult: model.WorkerExecution{ID: "exec-poll-cancel"}}
				eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-poll-cancel", Status: model.ExecStatusRunning}}
				return mgr, eq
			},
			instruction:    "long task",
			taskID:         "task-poll-cancel",
			messageID:      "msg-poll-cancel",
			wantWorkerName: "w1",
			cancelAt: func(t *testing.T, mgr task.ExecutionManager) {
				// Wait for ExecuteWorker so the dispatcher has moved into waitForResult's poll loop.
				require.True(t, waitForExecCount(mgr.(*mockExecManager), 1, 2*time.Second), "ExecuteWorker was not called within timeout")
			},
		},
		{
			name: "DuringResolve",
			setup: func() (task.ExecutionManager, task.ExecutionQuerier) {
				mgr := &cancelTrackingExecManager{cancelCount: new(int64)}
				eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-tracked", Status: model.ExecStatusCompleted}}
				return mgr, eq
			},
			instruction: "blocked task",
			taskID:      "task-resolve-cancel",
			messageID:   "msg-resolve-cancel",
			cancelAt: func(t *testing.T, mgr task.ExecutionManager) {
				// cancelTrackingExecManager blocks inside ExecuteWorker itself, so there is no
				// observable signal to wait on; give resolveExecution time to start.
				time.Sleep(50 * time.Millisecond)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr, eq := tc.setup()
			fn := &mockFailureNotifier{}
			d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(), task.WithFailureNotifier(fn))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go d.Run(ctx)

			t1 := immediateTask("s1", "w1", tc.instruction)
			t1.TaskID = tc.taskID
			t1.MessageID = tc.messageID
			in <- t1

			tc.cancelAt(t, mgr)

			require.NoError(t, d.CancelTask(context.Background(), tc.taskID))

			require.True(t, fn.waitForCancelCall(2*time.Second), "expected NotifyTaskCancelled to be called, but it was not")
			fn.mu.Lock()
			defer fn.mu.Unlock()
			got := fn.cancelCalls[0]
			assert.Equal(t, tc.messageID, got.messageID)
			if tc.wantWorkerName != "" {
				assert.Equal(t, tc.wantWorkerName, got.workerName)
			}
		})
	}
}

func TestTaskDispatcher_PollError_WithCancel_KillsProcess(t *testing.T) {
	execCalled := make(chan struct{}, 1)
	var cancelCount int64
	mgr := &quickCancelExecManager{
		execCalled:  execCalled,
		cancelCount: &cancelCount,
	}

	gate := make(chan struct{})
	eq := &cancelGateQuerier{gate: gate}

	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "some task")
	t1.TaskID = "task-poll-err"
	in <- t1

	// Wait until ExecuteWorker is called, meaning waitForResult has started and is now
	// blocked on the first GetByID call inside cancelGateQuerier.
	select {
	case <-execCalled:
	case <-time.After(2 * time.Second):
		require.Fail(t, "ExecuteWorker was not called within timeout")
	}

	// Cancel the task — sends to cancelCh.
	require.NoError(t, d.CancelTask(context.Background(), "task-poll-err"))

	// Give the Run loop time to process the cancel (handleCancel → cancel() → taskCtx done).
	time.Sleep(50 * time.Millisecond)

	// Unblock the querier: waitForResult now gets a DB error with ctx already done.
	close(gate)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&cancelCount) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotZero(t, atomic.LoadInt64(&cancelCount), "expected CancelExecution to be called when poll error occurs after cancel")
}
