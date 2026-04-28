package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildInstruction_WithPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz"}}`
	})
	task := DispatchTask{
		TaskID:    "task-1",
		MessageID: "msg-1",
		ReplyTo: platform.InboundMessage{
			Platform: "testplatform",
			Raw:      "any-raw",
		},
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if !strings.Contains(got, `"platform_context"`) {
		t.Errorf("expected platform_context in task_meta, got: %q", got)
	}
	if !strings.Contains(got, `"ou_abc"`) {
		t.Errorf("expected open_id value in task_meta, got: %q", got)
	}
	if !strings.Contains(got, "do something") {
		t.Errorf("expected instruction in output, got: %q", got)
	}
}

func TestBuildInstruction_NoPlatformContext(t *testing.T) {
	task := DispatchTask{
		TaskID:      "task-1",
		MessageID:   "msg-1",
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context should be omitted when empty, got: %q", got)
	}
}

// --- fakes for dispatcher internal tests ---

type fakeTaskQuerier struct {
	tasks map[string]model.Task
}

func newFakeTaskQuerier(tasks map[string]model.Task) *fakeTaskQuerier {
	return &fakeTaskQuerier{tasks: tasks}
}

func (f *fakeTaskQuerier) GetByID(_ context.Context, id string) (model.Task, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return model.Task{}, fmt.Errorf("task %s not found", id)
}

func (f *fakeTaskQuerier) ListByRoot(_ context.Context, rootID string) ([]model.Task, error) {
	var out []model.Task
	for _, t := range f.tasks {
		if t.RootTaskID == rootID || t.ID == rootID {
			out = append(out, t)
		}
	}
	return out, nil
}

type fakeGroupLookup struct {
	group   model.Group
	members []model.MemberBrief
}

func (f *fakeGroupLookup) GetByID(_ string) (model.Group, error) {
	return f.group, nil
}

func (f *fakeGroupLookup) ListMembers(_ string) ([]model.MemberBrief, error) {
	return f.members, nil
}

type fakeTaskStore struct{}

func (fakeTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (fakeTaskStore) CompleteTask(_ context.Context, _ string) error        { return nil }
func (fakeTaskStore) FailTask(_ context.Context, _ string) error            { return nil }
func (fakeTaskStore) CancelTask(_ context.Context, _ string) error          { return nil }

type fakeExecQuerier struct {
	result model.WorkerExecution
}

func (f *fakeExecQuerier) GetByID(_ string) (model.WorkerExecution, error) {
	return f.result, nil
}

type fakeExecMgr struct {
	mu           sync.Mutex
	instructions []string
	result       model.WorkerExecution
}

func newFakeExecMgr() *fakeExecMgr { return &fakeExecMgr{} }

func (f *fakeExecMgr) ExecuteWorker(_ context.Context, _, instruction, _ string, _ bool) (model.WorkerExecution, error) {
	f.mu.Lock()
	f.instructions = append(f.instructions, instruction)
	f.mu.Unlock()
	return f.result, nil
}

func (f *fakeExecMgr) CancelExecution(_ context.Context, _ string) error { return nil }

func (f *fakeExecMgr) lastInstruction() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.instructions) == 0 {
		return ""
	}
	return f.instructions[len(f.instructions)-1]
}

type fakeSessStore struct{}

func (fakeSessStore) GetSessionContextForEngine(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (fakeSessStore) UpsertSessionContext(_ context.Context, _, _, _, _ string) error { return nil }
func (fakeSessStore) DeleteSessionContextForEngine(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (fakeSessStore) ClearSessionContexts(_ context.Context, _, _ string) error { return nil }

// TestGroupPersonaInjection verifies that a group task causes group persona to be injected.
func TestGroupPersonaInjection(t *testing.T) {
	groupID := "g1"
	taskID := "root1"

	fq := newFakeTaskQuerier(map[string]model.Task{
		taskID: {
			ID:        taskID,
			WorkerID:  groupID,
			AgentKind: model.AgentKindGroup,
			Status:    model.TaskStatusRunning,
			MessageID: "m1",
		},
	})

	gl := &fakeGroupLookup{
		group: model.Group{
			ID:          groupID,
			Name:        "data-team",
			Description: "ETL team",
		},
		members: []model.MemberBrief{
			{ID: "w1", Name: "alice"},
			{ID: "w2", Name: "bob"},
		},
	}

	execMgr := newFakeExecMgr()
	execMgr.result = model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}

	eq := &fakeExecQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

	in := make(chan DispatchTask, 4)
	d := New(execMgr, fakeTaskStore{}, fakeSessStore{}, eq, in, enginecfg.NewStore("claude"),
		WithTaskQuerier(fq),
		WithGroupLookup(gl),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- DispatchTask{
		TaskID:      taskID,
		WorkerID:    groupID,
		Instruction: "do group work",
		TaskType:    model.TaskTypeImmediate,
		MessageID:   "m1",
	}

	// Wait for ExecuteWorker to be called.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if inst := execMgr.lastInstruction(); inst != "" {
			if !strings.Contains(inst, "<group_persona>") {
				t.Errorf("expected <group_persona> in prompt, got:\n%s", inst)
			}
			if !strings.Contains(inst, "alice") {
				t.Errorf("expected member 'alice' in prompt, got:\n%s", inst)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ExecuteWorker was not called within timeout")
}
