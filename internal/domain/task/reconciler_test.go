package task_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type reconTaskStore struct {
	mu            sync.Mutex
	running       []model.Task
	completed     []string
	failed        []string
	completeErr   error
	failErr       error
	listCalled    int
	listReturnErr error
}

func (s *reconTaskStore) List(_ context.Context, f store.TaskFilter) ([]model.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalled++
	if s.listReturnErr != nil {
		return nil, s.listReturnErr
	}
	if f.Status != model.TaskStatusRunning {
		return nil, nil
	}
	out := make([]model.Task, len(s.running))
	copy(out, s.running)
	return out, nil
}

func (s *reconTaskStore) CompleteTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completeErr != nil {
		return s.completeErr
	}
	s.completed = append(s.completed, taskID)
	return nil
}

func (s *reconTaskStore) FailTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != nil {
		return s.failErr
	}
	s.failed = append(s.failed, taskID)
	return nil
}

type reconExecStore struct {
	mu sync.Mutex
	// byTask holds the latest execution recorded for each taskID.
	byTask                  map[string]model.WorkerExecution
	abandoned               []string
	listByTaskIDsCalls      int
	runningExecIDsCalls     int
}

func (s *reconExecStore) ListByTaskIDs(_ context.Context, taskIDs []string, _ int) (map[string][]model.WorkerExecution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listByTaskIDsCalls++
	out := make(map[string][]model.WorkerExecution, len(taskIDs))
	for _, id := range taskIDs {
		if e, ok := s.byTask[id]; ok {
			out[id] = []model.WorkerExecution{e}
		}
	}
	return out, nil
}

func (s *reconExecStore) RunningExecIDsByTaskIDs(_ context.Context, taskIDs []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runningExecIDsCalls++
	out := make(map[string]string, len(taskIDs))
	for _, id := range taskIDs {
		if e, ok := s.byTask[id]; ok && e.Status == model.ExecStatusRunning {
			out[id] = e.ID
		}
	}
	return out, nil
}

func (s *reconExecStore) MarkAbandoned(_ context.Context, id, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abandoned = append(s.abandoned, id)
	for taskID, e := range s.byTask {
		if e.ID == id {
			e.Status = model.ExecStatusFailed
			s.byTask[taskID] = e
		}
	}
	return true, nil
}

func newReconcilerForTest(taskStore *reconTaskStore, execStore *reconExecStore, alive func(int) bool) *task.Reconciler {
	r := task.NewReconciler(taskStore, execStore, time.Second)
	if alive != nil {
		task.SetProcessAliveForTest(r, alive)
	}
	return r
}

func TestReconciler_CompletesStaleRunningTask(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{{ID: "t1", Status: model.TaskStatusRunning}},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{
		"t1": {ID: "e1", TaskID: "t1", Status: model.ExecStatusCompleted},
	}}
	r := newReconcilerForTest(tasks, execs, nil)
	task.ReconcileForTest(r, context.Background())

	if len(tasks.completed) != 1 || tasks.completed[0] != "t1" {
		t.Fatalf("expected t1 completed, got %v", tasks.completed)
	}
	if len(tasks.failed) != 0 {
		t.Fatalf("expected no failed, got %v", tasks.failed)
	}
}

func TestReconciler_FailsStaleRunningTask(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{{ID: "t1", Status: model.TaskStatusRunning}},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{
		"t1": {ID: "e1", TaskID: "t1", Status: model.ExecStatusFailed},
	}}
	r := newReconcilerForTest(tasks, execs, nil)
	task.ReconcileForTest(r, context.Background())

	if len(tasks.failed) != 1 || tasks.failed[0] != "t1" {
		t.Fatalf("expected t1 failed, got %v", tasks.failed)
	}
}

func TestReconciler_SweepsOrphanedExecWhenProcessDead(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{{ID: "t1", Status: model.TaskStatusRunning}},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{
		"t1": {ID: "e1", TaskID: "t1", Status: model.ExecStatusRunning, AIProcessPID: 12345},
	}}
	r := newReconcilerForTest(tasks, execs, func(_ int) bool { return false })
	task.ReconcileForTest(r, context.Background())

	if len(execs.abandoned) != 1 || execs.abandoned[0] != "e1" {
		t.Fatalf("expected e1 abandoned, got %v", execs.abandoned)
	}
	if len(tasks.failed) != 1 || tasks.failed[0] != "t1" {
		t.Fatalf("expected t1 failed, got %v", tasks.failed)
	}
}

func TestReconciler_LeavesLiveRunningExecAlone(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{{ID: "t1", Status: model.TaskStatusRunning}},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{
		"t1": {ID: "e1", TaskID: "t1", Status: model.ExecStatusRunning, AIProcessPID: 12345},
	}}
	r := newReconcilerForTest(tasks, execs, func(_ int) bool { return true })
	task.ReconcileForTest(r, context.Background())

	if len(execs.abandoned) != 0 || len(tasks.failed) != 0 || len(tasks.completed) != 0 {
		t.Fatalf("expected no changes, got abandoned=%v failed=%v completed=%v",
			execs.abandoned, tasks.failed, tasks.completed)
	}
}

func TestReconciler_SkipsTaskWithoutExecution(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{{ID: "t1", Status: model.TaskStatusRunning}},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{}}
	r := newReconcilerForTest(tasks, execs, nil)
	task.ReconcileForTest(r, context.Background())

	if len(tasks.completed) != 0 || len(tasks.failed) != 0 {
		t.Fatalf("expected no changes for unstarted task, got completed=%v failed=%v",
			tasks.completed, tasks.failed)
	}
}

func TestReconciler_BatchesExecStoreCalls(t *testing.T) {
	tasks := &reconTaskStore{
		running: []model.Task{
			{ID: "t1", Status: model.TaskStatusRunning},
			{ID: "t2", Status: model.TaskStatusRunning},
			{ID: "t3", Status: model.TaskStatusRunning},
		},
	}
	execs := &reconExecStore{byTask: map[string]model.WorkerExecution{
		"t1": {ID: "e1", TaskID: "t1", Status: model.ExecStatusCompleted},
		"t2": {ID: "e2", TaskID: "t2", Status: model.ExecStatusFailed},
		"t3": {ID: "e3", TaskID: "t3", Status: model.ExecStatusRunning, AIProcessPID: 12345},
	}}
	r := newReconcilerForTest(tasks, execs, func(_ int) bool { return true })
	task.ReconcileForTest(r, context.Background())

	if execs.listByTaskIDsCalls != 1 {
		t.Fatalf("expected 1 ListByTaskIDs call, got %d", execs.listByTaskIDsCalls)
	}
	if execs.runningExecIDsCalls != 1 {
		t.Fatalf("expected 1 RunningExecIDsByTaskIDs call, got %d", execs.runningExecIDsCalls)
	}
}
