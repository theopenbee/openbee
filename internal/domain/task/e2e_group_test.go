package task

import (
	"context"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/group"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// TestE2E_GroupHappyPath exercises the full group task lifecycle:
// 1. Group root task is created with agent_kind=group
// 2. Group calls MarkWaitingSubtasks (suspend)
// 3. Two subtasks are created and marked completed
// 4. notifyParentOnSubtaskTerminal fires a resume event
// 5. The root task is completed via MarkSuccess
func TestE2E_GroupHappyPath(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := store.NewWorkerStore(db)
	gs := store.NewGroupStore(db)
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	ss := store.NewSessionStore(db)

	ctx := context.Background()

	// Create two member workers + a group
	w1, err := ws.Create(model.Worker{Name: "alice", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker alice: %v", err)
	}
	w2, err := ws.Create(model.Worker{Name: "bob", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker bob: %v", err)
	}
	gm := group.NewManager(t.TempDir(), gs, ws, ts, nil, nil, nil)
	g, err := gm.CreateGroup(group.CreateGroupParams{Name: "data-team", Description: "ETL"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := gm.AddMember(g.ID, w1.ID); err != nil {
		t.Fatalf("add w1: %v", err)
	}
	if err := gm.AddMember(g.ID, w2.ID); err != nil {
		t.Fatalf("add w2: %v", err)
	}

	// Seed a platform message
	msgID := "msg-1"
	_, err = ms.Create(ctx, msgID, "session-1", "feishu", "@data-team please fetch X", "", "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_ = ss

	// Step 1: create the group root task
	rootID, err := ts.Create(ctx, model.Task{
		MessageID:   msgID,
		WorkerID:    g.ID,
		Instruction: "coordinate fetching X",
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusRunning,
		AgentKind:   model.AgentKindGroup,
	})
	if err != nil {
		t.Fatalf("create root task: %v", err)
	}

	// Step 2: Group calls Suspend → marks root as waiting_subtasks
	if err := ts.MarkWaitingSubtasks(ctx, rootID); err != nil {
		t.Fatalf("MarkWaitingSubtasks: %v", err)
	}
	root, _ := ts.GetByID(ctx, rootID)
	if root.Status != model.TaskStatusWaitingSubtasks {
		t.Errorf("expected waiting_subtasks, got %s", root.Status)
	}

	// Step 3: Create two subtasks
	sub1ID, err := ts.Create(ctx, model.Task{
		MessageID:    msgID,
		WorkerID:     w1.ID,
		Instruction:  "fetch part 1",
		Type:         model.TaskTypeImmediate,
		Status:       model.TaskStatusPending,
		ParentTaskID: rootID,
		RootTaskID:   rootID,
		AgentKind:    model.AgentKindWorker,
	})
	if err != nil {
		t.Fatalf("create sub1: %v", err)
	}
	sub2ID, err := ts.Create(ctx, model.Task{
		MessageID:    msgID,
		WorkerID:     w2.ID,
		Instruction:  "fetch part 2",
		Type:         model.TaskTypeImmediate,
		Status:       model.TaskStatusPending,
		ParentTaskID: rootID,
		RootTaskID:   rootID,
		AgentKind:    model.AgentKindWorker,
	})
	if err != nil {
		t.Fatalf("create sub2: %v", err)
	}

	// Step 4: Complete the subtasks and verify dispatcher resume logic
	subtask1, _ := ts.GetByID(ctx, sub1ID)
	subtask2, _ := ts.GetByID(ctx, sub2ID)

	if err := ts.CompleteTask(ctx, sub1ID); err != nil {
		t.Fatalf("complete sub1: %v", err)
	}
	if err := ts.CompleteTask(ctx, sub2ID); err != nil {
		t.Fatalf("complete sub2: %v", err)
	}

	// Wire a TaskQuerier and test notifyParentOnSubtaskTerminal
	inCh := make(chan DispatchTask, 16)
	d := New(newFakeExecMgr(), fakeTaskStore{}, fakeSessStore{}, &fakeExecQuerier{}, inCh, nil,
		WithTaskQuerier(ts),
	)

	// Simulate sub-task 1 completing — should enqueue a resume for the root
	d.notifyParentOnSubtaskTerminal(ctx, DispatchTask{
		TaskID:     sub1ID,
		WorkerID:   subtask1.WorkerID,
		SessionKey: "session-1",
	})

	var resumeEvent DispatchTask
	select {
	case resumeEvent = <-d.subtaskEventCh:
	case <-time.After(time.Second):
		t.Fatal("no resume event for sub1 completion")
	}
	if resumeEvent.TaskID != rootID {
		t.Errorf("expected resume for rootID=%s, got %s", rootID, resumeEvent.TaskID)
	}

	// Simulate sub-task 2 completing
	d.notifyParentOnSubtaskTerminal(ctx, DispatchTask{
		TaskID:     sub2ID,
		WorkerID:   subtask2.WorkerID,
		SessionKey: "session-1",
	})
	select {
	case ev := <-d.subtaskEventCh:
		if ev.TaskID != rootID {
			t.Errorf("expected resume for rootID=%s, got %s", rootID, ev.TaskID)
		}
	case <-time.After(time.Second):
		t.Fatal("no resume event for sub2 completion")
	}

	// Step 5: Group completes the root task
	if err := ts.CompleteTask(ctx, rootID); err != nil {
		t.Fatalf("CompleteTask root: %v", err)
	}
	finalRoot, _ := ts.GetByID(ctx, rootID)
	if finalRoot.Status != model.TaskStatusCompleted {
		t.Errorf("expected root completed, got %s", finalRoot.Status)
	}

	// Verify all tasks completed
	allTasks, _ := ts.ListByRoot(ctx, rootID)
	for _, tk := range allTasks {
		if tk.Status != model.TaskStatusCompleted {
			t.Errorf("task %s not completed: %s", tk.ID, tk.Status)
		}
	}
}
