package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestUsageStore_Insert_And_Get(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()

	us := NewUsageStore(db)
	execID := uuid.New().String()
	record := &model.UsageRecord{
		ID:                  uuid.New().String(),
		ExecutionID:         execID,
		Model:               "claude-sonnet-4-6",
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     300,
		TotalTokens:         650,
		CostUSD:             0.42,
		SyncedAt:            time.Now().UnixMilli(),
	}

	require.NoError(t, us.Insert(record))

	got, err := us.GetByExecutionID(execID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, execID, got.ExecutionID)
	assert.Equal(t, "claude-sonnet-4-6", got.Model)
	assert.Equal(t, int64(100), got.InputTokens)
	assert.Equal(t, int64(650), got.TotalTokens)
	assert.InDelta(t, 0.42, got.CostUSD, 0.001)
}

func TestUsageStore_Insert_Idempotent(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()

	us := NewUsageStore(db)
	execID := uuid.New().String()
	record := &model.UsageRecord{
		ID:          uuid.New().String(),
		ExecutionID: execID,
		SyncedAt:    time.Now().UnixMilli(),
	}

	require.NoError(t, us.Insert(record))
	require.NoError(t, us.Insert(record)) // second call must not error
}

func TestUsageStore_GetByExecutionID_NotFound(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()

	us := NewUsageStore(db)
	got, err := us.GetByExecutionID("no-such-id")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUsageStore_ListUnsynced(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())
	us := NewUsageStore(db)

	w, _ := ws.Create(model.Worker{Name: "bot", WorkDir: "/tmp"})

	// Create 3 completed executions with a log_path
	for i := range 3 {
		exec, _ := es.Create(w.ID, "task", uuid.New().String())
		db.Exec(`UPDATE bee_executions SET status='completed', log_path='/tmp/fake.log' WHERE id=?`, exec.ID)
		_ = i
	}

	// Create 1 failed execution with log_path
	exec4, _ := es.Create(w.ID, "task", uuid.New().String())
	db.Exec(`UPDATE bee_executions SET status='failed', log_path='/tmp/fake.log' WHERE id=?`, exec4.ID)

	// Create 1 pending execution (should NOT appear)
	es.Create(w.ID, "task", uuid.New().String())

	unsynced, err := us.ListUnsynced(10)
	require.NoError(t, err)
	assert.Len(t, unsynced, 4)

	// Sync one of them, it should disappear from the list
	require.NoError(t, us.Insert(&model.UsageRecord{
		ID:          uuid.New().String(),
		ExecutionID: unsynced[0].ID,
		SyncedAt:    time.Now().UnixMilli(),
	}))

	unsynced2, err := us.ListUnsynced(10)
	require.NoError(t, err)
	assert.Len(t, unsynced2, 3)
}
