package store

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupTestDB(t *testing.T) *WorkerStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewWorkerStore(db)
}

func TestWorkerStore_Create(t *testing.T) {
	s := setupTestDB(t)
	w := model.Worker{
		Name:        "TestBot",
		Description: "A test worker",
		WorkDir:     "/tmp/testbot",
	}
	created, err := s.Create(w)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Status != model.WorkerStatusIdle {
		t.Errorf("expected status idle, got %s", created.Status)
	}
}

func TestWorkerStore_GetByID(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{
		Name: "Bot1", WorkDir: "/tmp/bot1",
	})
	got, err := s.GetByID(w.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Bot1" {
		t.Errorf("expected Bot1, got %s", got.Name)
	}
}

func TestWorkerStore_List(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "A", WorkDir: "/tmp/a"})
	s.Create(model.Worker{Name: "B", WorkDir: "/tmp/b"})
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 workers, got %d", len(list))
	}
}

func TestWorkerStore_Update(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{Name: "Old", WorkDir: "/tmp/old"})
	w.Name = "New"
	updated, err := s.Update(w)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("expected New, got %s", updated.Name)
	}
}

func TestWorkerStore_Delete(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{Name: "Del", WorkDir: "/tmp/del"})
	if err := s.Delete(w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.GetByID(w.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestWorkerStore_CountByStatus(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)

	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','a','/tmp','idle',0,0)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','b','/tmp','idle',0,0)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w3','c','/tmp','working',0,0)`)

	counts, err := ws.CountByStatus()
	if err != nil {
		t.Fatal(err)
	}
	if counts["idle"] != 2 || counts["working"] != 1 {
		t.Errorf("unexpected counts: %v", counts)
	}
}
