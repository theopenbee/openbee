package group

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewManager(
		t.TempDir(), // groupBaseDir
		store.NewGroupStore(db),
		store.NewWorkerStore(db),
		store.NewTaskStore(db),
		nil, // engines map (not exercised in CRUD tests)
		nil, // engineCfg
		nil, // botNames
	)
}

func seedMessage(t *testing.T, m *Manager) string {
	t.Helper()
	id := uuid.New().String()
	_, err := m.taskStore.DB().Exec(
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
         VALUES (?, 'sk', 'feishu', 'hi', '', '', 1, 1, 1)`, id)
	if err != nil {
		t.Fatalf("seedMessage: %v", err)
	}
	return id
}

func testCtx() context.Context { return context.Background() }

func TestManager_CreateGroup(t *testing.T) {
	m := newTestManager(t)
	g, err := m.CreateGroup(CreateGroupParams{Name: "data-team"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.ID == "" || g.WorkDir == "" {
		t.Errorf("expected ID and WorkDir, got %+v", g)
	}
}

func TestManager_CreateGroup_NameCollision_WithWorker(t *testing.T) {
	m := newTestManager(t)
	_, _ = m.workerStore.Create(model.Worker{Name: "alpha", WorkDir: "/tmp/w"})
	_, err := m.CreateGroup(CreateGroupParams{Name: "alpha"})
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation on collision with worker, got %v", err)
	}
}

func TestManager_DeleteGroup_RejectsActiveRootTask(t *testing.T) {
	m := newTestManager(t)
	g, _ := m.CreateGroup(CreateGroupParams{Name: "g"})
	// create a fake active root task for this group
	msgID := seedMessage(t, m)
	_, _ = m.taskStore.Create(testCtx(), model.Task{
		MessageID: msgID, WorkerID: g.ID, Instruction: "x",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		AgentKind: model.AgentKindGroup,
	})
	err := m.DeleteGroup(g.ID, false)
	if err == nil {
		t.Error("expected delete rejection while a root task is running")
	}
}

func TestManager_AddRemoveMember(t *testing.T) {
	m := newTestManager(t)
	g, _ := m.CreateGroup(CreateGroupParams{Name: "g"})
	w, _ := m.workerStore.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
	if err := m.AddMember(g.ID, w.ID); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	members, _ := m.groupStore.ListMembers(g.ID)
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
	if err := m.RemoveMember(g.ID, w.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
}
