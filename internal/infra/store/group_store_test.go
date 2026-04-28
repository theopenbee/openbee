package store

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupGroupTestDB(t *testing.T) (*GroupStore, *WorkerStore) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewGroupStore(db), NewWorkerStore(db)
}

func TestGroupStore_Create(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	g, err := gs.Create(model.Group{Name: "data-team", WorkDir: "/tmp/g1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.ID == "" {
		t.Error("expected non-empty ID")
	}
	if g.Status != model.WorkerStatusIdle {
		t.Errorf("expected idle, got %s", g.Status)
	}
	if g.EngineArgs != "{}" {
		t.Errorf("expected default engine_args '{}', got %q", g.EngineArgs)
	}
}

func TestGroupStore_GetByID(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "ops", WorkDir: "/tmp/g2"})
	got, err := gs.GetByID(g.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "ops" {
		t.Errorf("expected ops, got %s", got.Name)
	}
}

func TestGroupStore_GetByName(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	_, _ = gs.Create(model.Group{Name: "Alpha", WorkDir: "/tmp/g3"})
	got, err := gs.GetByName("alpha") // case-insensitive
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "Alpha" {
		t.Errorf("expected Alpha, got %s", got.Name)
	}
}

func TestGroupStore_ExistsByName(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "Beta", WorkDir: "/tmp/g4"})
	yes, err := gs.ExistsByName("BETA", "")
	if err != nil || !yes {
		t.Fatalf("ExistsByName(BETA, ''): got (%v, %v), want (true, nil)", yes, err)
	}
	no, _ := gs.ExistsByName("BETA", g.ID) // exclude self
	if no {
		t.Errorf("ExistsByName(BETA, self): expected false")
	}
}

func TestGroupStore_List(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	_, _ = gs.Create(model.Group{Name: "g1", WorkDir: "/tmp/g1"})
	_, _ = gs.Create(model.Group{Name: "g2", WorkDir: "/tmp/g2"})
	list, err := gs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestGroupStore_Update(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "old", WorkDir: "/tmp/g1"})
	g.Name = "new"
	g.Description = "updated"
	out, err := gs.Update(g)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.Name != "new" || out.Description != "updated" {
		t.Errorf("update did not stick: %+v", out)
	}
}

func TestGroupStore_Delete(t *testing.T) {
	gs, _ := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "tmp", WorkDir: "/tmp/g1"})
	if err := gs.Delete(g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := gs.GetByID(g.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestGroupStore_AddMember(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

	if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	members, err := gs.ListMembers(g.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 || members[0].ID != w.ID {
		t.Errorf("expected 1 member with id %s, got %+v", w.ID, members)
	}
}

func TestGroupStore_AddMember_Idempotent(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

	if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
		t.Fatalf("AddMember 1: %v", err)
	}
	if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
		t.Fatalf("AddMember 2 (idempotent): %v", err)
	}
}

func TestGroupStore_RemoveMember(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
	_ = gs.AddMember(g.ID, w.ID, "member")

	if err := gs.RemoveMember(g.ID, w.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	members, _ := gs.ListMembers(g.ID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after remove, got %d", len(members))
	}
}

func TestGroupStore_IsMember(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

	yes, _ := gs.IsMember(g.ID, w.ID)
	if yes {
		t.Error("expected false before AddMember")
	}
	_ = gs.AddMember(g.ID, w.ID, "member")
	yes, _ = gs.IsMember(g.ID, w.ID)
	if !yes {
		t.Error("expected true after AddMember")
	}
}

func TestGroupStore_ListGroupsForWorker(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g1, _ := gs.Create(model.Group{Name: "g1", WorkDir: "/tmp/g1"})
	g2, _ := gs.Create(model.Group{Name: "g2", WorkDir: "/tmp/g2"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
	_ = gs.AddMember(g1.ID, w.ID, "member")
	_ = gs.AddMember(g2.ID, w.ID, "member")

	groups, err := gs.ListGroupsForWorker(w.ID)
	if err != nil {
		t.Fatalf("ListGroupsForWorker: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroupStore_DeleteCascadesMemberships(t *testing.T) {
	gs, ws := setupGroupTestDB(t)
	g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
	w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
	_ = gs.AddMember(g.ID, w.ID, "member")

	if err := gs.Delete(g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	groups, _ := gs.ListGroupsForWorker(w.ID)
	if len(groups) != 0 {
		t.Errorf("expected memberships cleared on group delete, got %d", len(groups))
	}
}
