package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// newTestDB opens a migrated SQLite database under t.TempDir, applies the seed
// statements in order, and closes the database when the test ends.
func newTestDB(t *testing.T, seeds ...string) *sql.DB {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for _, q := range seeds {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	return db
}

func seedWorkerSQL(id string) string {
	return fmt.Sprintf(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('%s','%s','/','idle',1,1)`, id, id)
}

func seedMessageSQL(id, sessionKey string) string {
	return fmt.Sprintf(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('%s','%s','feishu','hi','','',1,1,1)`, id, sessionKey)
}

// seedTask creates task, defaulting unset fields to message m1, worker w1,
// instruction "go", an immediate pending task, and the current time.
func seedTask(t *testing.T, ts *TaskStore, task model.Task) string {
	t.Helper()
	now := time.Now().UnixMilli()
	if task.MessageID == "" {
		task.MessageID = "m1"
	}
	if task.WorkerID == "" {
		task.WorkerID = "w1"
	}
	if task.Instruction == "" {
		task.Instruction = "go"
	}
	if task.Type == "" {
		task.Type = model.TaskTypeImmediate
	}
	if task.Status == "" {
		task.Status = model.TaskStatusPending
	}
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	if task.UpdatedAt == 0 {
		task.UpdatedAt = now
	}
	id, err := ts.Create(context.Background(), task)
	require.NoError(t, err)
	return id
}
