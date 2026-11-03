package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupTestDB(t *testing.T) *WorkerStore {
	t.Helper()
	return NewWorkerStore(newTestDB(t))
}

func TestWorkerStore_Create(t *testing.T) {
	s := setupTestDB(t)
	w := model.Worker{
		Name:        "TestBot",
		Description: "A test worker",
		WorkDir:     "/tmp/testbot",
	}
	created, err := s.Create(w)
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, model.WorkerStatusIdle, created.Status)
}

func TestWorkerStore_GetByID(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{
		Name: "Bot1", WorkDir: "/tmp/bot1",
	})
	got, err := s.GetByID(w.ID)
	require.NoError(t, err)
	assert.Equal(t, "Bot1", got.Name)
}

func TestWorkerStore_List(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "A", WorkDir: "/tmp/a"})
	s.Create(model.Worker{Name: "B", WorkDir: "/tmp/b"})
	list, err := s.List()
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestWorkerStore_Update(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{Name: "Old", WorkDir: "/tmp/old"})
	w.Name = "New"
	updated, err := s.Update(w)
	require.NoError(t, err)
	assert.Equal(t, "New", updated.Name)
}

func TestWorkerStore_Delete(t *testing.T) {
	s := setupTestDB(t)
	w, _ := s.Create(model.Worker{Name: "Del", WorkDir: "/tmp/del"})
	require.NoError(t, s.Delete(w.ID))
	_, err := s.GetByID(w.ID)
	assert.Error(t, err)
}

func TestWorkerStore_GetByName_ExactMatch(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "天天", WorkDir: "/tmp/tt"})

	got, err := s.GetByName("天天")
	require.NoError(t, err)
	assert.Equal(t, "天天", got.Name)
}

func TestWorkerStore_GetByName_CaseInsensitive(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "Alice", WorkDir: "/tmp/alice"})

	got, err := s.GetByName("alice")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)

	got2, err := s.GetByName("ALICE")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got2.Name)
}

func TestWorkerStore_GetByName_NotFound(t *testing.T) {
	s := setupTestDB(t)

	_, err := s.GetByName("nobody")
	require.Error(t, err)
}

func TestWorkerStore_GetByName_DuplicateName_ReturnsEarliest(t *testing.T) {
	s := setupTestDB(t)
	first, _ := s.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot1"})
	s.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot2"})

	got, err := s.GetByName("bot")
	require.NoError(t, err)
	assert.Equal(t, first.ID, got.ID)
}

func TestWorkerStore_EngineField(t *testing.T) {
	dir := t.TempDir()
	ws := NewWorkerStore(newTestDB(t))

	w, err := ws.Create(model.Worker{
		Name:    "test-worker",
		WorkDir: dir,
		Engine:  "codex",
	})
	require.NoError(t, err)
	assert.Equal(t, "codex", w.Engine)

	got, err := ws.GetByID(w.ID)
	require.NoError(t, err)
	assert.Equal(t, "codex", got.Engine, "after get")

	updated, err := ws.Update(model.Worker{
		ID:      w.ID,
		Name:    w.Name,
		WorkDir: w.WorkDir,
		Engine:  "pi",
	})
	require.NoError(t, err)
	assert.Equal(t, "pi", updated.Engine, "after update")
}

func TestWorkerStore_CountByStatus(t *testing.T) {
	db := newTestDB(t,
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','a','/tmp','idle',0,0)`,
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','b','/tmp','idle',0,0)`,
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w3','c','/tmp','working',0,0)`,
	)
	ws := NewWorkerStore(db)

	counts, err := ws.CountByStatus()
	require.NoError(t, err)
	assert.Equal(t, 2, counts["idle"])
	assert.Equal(t, 1, counts["working"])
}

func TestWorkerStore_ListNames(t *testing.T) {
	s := setupTestDB(t)

	// Empty store returns empty slice without error
	names, err := s.ListNames()
	require.NoError(t, err)
	assert.Empty(t, names)

	// Seed two workers
	s.Create(model.Worker{Name: "Alpha"})
	s.Create(model.Worker{Name: "Beta"})

	names, err = s.ListNames()
	require.NoError(t, err)
	assert.Len(t, names, 2)

	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	assert.True(t, nameSet["Alpha"] && nameSet["Beta"], "expected Alpha and Beta in %v", names)
}
