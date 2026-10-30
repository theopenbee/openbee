package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type ExecutionStore struct {
	db      *sql.DB
	logsDir string
}

func NewExecutionStore(db *sql.DB, logsDir string) *ExecutionStore {
	return &ExecutionStore{db: db, logsDir: logsDir}
}

// ExecutionCreate carries the inputs for ExecutionStore.Create.
// WorkerID is empty for bee-owned executions; TaskID is empty for raw exec rows
// not driven by a task.
type ExecutionCreate struct {
	WorkerID     string
	TaskID       string
	TriggerInput string
	SessionID    string
	Engine       string
}

// Create inserts a new execution. An empty WorkerID inserts NULL for worker_id
// and marks the row as bee-owned; otherwise the execution is attributed to that
// worker. TaskID may be empty for bee-owned executions.
func (s *ExecutionStore) Create(c ExecutionCreate) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		TaskID:       c.TaskID,
		SessionID:    c.SessionID,
		Engine:       c.Engine,
		TriggerInput: c.TriggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	if c.WorkerID != "" {
		wid := c.WorkerID
		exec.WorkerID = &wid
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.TaskID, exec.WorkerID, exec.SessionID, exec.Engine, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert execution: %w", err)
	}
	return exec, nil
}

// inListChunkSize bounds how many ids we pack into a single IN (?,?,…) —
// SQLite's default SQLITE_MAX_VARIABLE_NUMBER is 999.
const inListChunkSize = 500

const execSelect = `
SELECT e.id, e.task_id, e.worker_id, e.session_id, e.engine, e.trigger_input, e.status, e.result, e.log_path,
       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
FROM bee_executions e
LEFT JOIN bee_workers w ON w.id = e.worker_id`

func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
	var e model.WorkerExecution
	err := scanner.Scan(&e.ID, &e.TaskID, &e.WorkerID, &e.SessionID, &e.Engine, &e.TriggerInput, &e.Status, &e.Result, &e.LogPath, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
	return e, err
}

func (s *ExecutionStore) GetByID(id string) (model.WorkerExecution, error) {
	row := s.db.QueryRow(execSelect+` WHERE e.id = ?`, id)
	e, err := scanExecution(row)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get execution: %w", err)
	}
	return e, nil
}

func (s *ExecutionStore) List() ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(execSelect + ` ORDER BY e.started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// GetRunningByWorkerID returns the currently running execution for a worker, or nil if none.
func (s *ExecutionStore) GetRunningByWorkerID(workerID string) (*model.WorkerExecution, error) {
	row := s.db.QueryRow(execSelect+` WHERE e.worker_id = ? AND e.status = ? LIMIT 1`, workerID, model.ExecStatusRunning)
	e, err := scanExecution(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running execution by worker: %w", err)
	}
	return &e, nil
}

// GetRunningByTaskID returns the currently running execution for a task, or nil if none.
func (s *ExecutionStore) GetRunningByTaskID(ctx context.Context, taskID string) (*model.WorkerExecution, error) {
	row := s.db.QueryRowContext(ctx, execSelect+` WHERE e.task_id = ? AND e.status = ? LIMIT 1`, taskID, model.ExecStatusRunning)
	e, err := scanExecution(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running execution by task: %w", err)
	}
	return &e, nil
}

// RunningExecIDsByTaskIDs returns a map of task_id -> running execution id for the given tasks.
func (s *ExecutionStore) RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(taskIDs))
	for start := 0; start < len(taskIDs); start += inListChunkSize {
		end := start + inListChunkSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		chunk := taskIDs[start:end]
		args := append([]any{model.ExecStatusRunning}, stringsToArgs(chunk)...)
		q := `SELECT task_id, id FROM bee_executions WHERE status = ? AND task_id IN (` + inPlaceholders(len(chunk)) + `)`
		if err := s.scanRunningExecIDs(ctx, q, args, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *ExecutionStore) scanRunningExecIDs(ctx context.Context, q string, args []any, out map[string]string) error {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("running exec ids by task ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, execID string
		if err := rows.Scan(&taskID, &execID); err != nil {
			return fmt.Errorf("scan running exec row: %w", err)
		}
		out[taskID] = execID
	}
	return rows.Err()
}

// ListByTaskIDs returns executions grouped by task_id, newest first within each
// task. Task ids with no executions are omitted from the result. When
// limitPerTask > 0, at most that many executions are returned per task;
// limitPerTask <= 0 returns all executions.
func (s *ExecutionStore) ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error) {
	out := make(map[string][]model.WorkerExecution, len(taskIDs))
	for start := 0; start < len(taskIDs); start += inListChunkSize {
		end := start + inListChunkSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		if err := s.listByTaskIDsChunk(ctx, taskIDs[start:end], limitPerTask, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *ExecutionStore) listByTaskIDsChunk(ctx context.Context, taskIDs []string, limitPerTask int, out map[string][]model.WorkerExecution) error {
	args := stringsToArgs(taskIDs)
	var q string
	if limitPerTask <= 0 {
		q = execSelect + ` WHERE e.task_id IN (` + inPlaceholders(len(taskIDs)) + `) ORDER BY e.started_at DESC, e.rowid DESC`
	} else {
		q = `WITH ranked AS (
		SELECT e.rowid AS exec_rowid,
		       ROW_NUMBER() OVER (PARTITION BY e.task_id ORDER BY e.started_at DESC, e.rowid DESC) AS rn
		FROM bee_executions e
		WHERE e.task_id IN (` + inPlaceholders(len(taskIDs)) + `)
	)
	SELECT e.id, e.task_id, e.worker_id, e.session_id, e.engine, e.trigger_input, e.status, e.result, e.log_path,
	       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
	FROM ranked r
	JOIN bee_executions e ON e.rowid = r.exec_rowid
	LEFT JOIN bee_workers w ON w.id = e.worker_id
	WHERE r.rn <= ?
	ORDER BY e.started_at DESC, e.rowid DESC`
		args = append(args, limitPerTask)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("list executions by task ids: %w", err)
	}
	defer rows.Close()
	execs, err := scanExecutions(rows)
	if err != nil {
		return err
	}
	for _, e := range execs {
		out[e.TaskID] = append(out[e.TaskID], e)
	}
	return nil
}

// ListByIDs returns executions whose id is in ids. Order is unspecified.
func (s *ExecutionStore) ListByIDs(ctx context.Context, ids []string) ([]model.WorkerExecution, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]model.WorkerExecution, 0, len(ids))
	for start := 0; start < len(ids); start += inListChunkSize {
		end := start + inListChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		q := execSelect + ` WHERE e.id IN (` + inPlaceholders(len(chunk)) + `)`
		rows, err := s.db.QueryContext(ctx, q, stringsToArgs(chunk)...)
		if err != nil {
			return nil, fmt.Errorf("list executions by ids: %w", err)
		}
		execs, err := scanExecutions(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, execs...)
	}
	return out, nil
}

// HasActiveBeeExecutions reports whether bee-owned executions (worker_id IS NULL)
// with status pending or running exist.
func (s *ExecutionStore) HasActiveBeeExecutions(ctx context.Context) (bool, error) {
	return s.hasActiveExecutions(ctx, "worker_id IS NULL")
}

// HasActiveExecutionsByWorkerID reports whether the given worker has any
// pending or running executions.
func (s *ExecutionStore) HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error) {
	return s.hasActiveExecutions(ctx, "worker_id = ?", workerID)
}

func (s *ExecutionStore) hasActiveExecutions(ctx context.Context, ownerWhere string, ownerArgs ...any) (bool, error) {
	args := append(ownerArgs, model.ExecStatusPending, model.ExecStatusRunning)
	q := `SELECT EXISTS(SELECT 1 FROM bee_executions WHERE ` + ownerWhere + ` AND status IN (?, ?))`
	var exists int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&exists)
	return exists == 1, err
}

func (s *ExecutionStore) UpdateStatus(id string, status model.ExecutionStatus) error {
	_, err := s.db.Exec(`UPDATE bee_executions SET status=? WHERE id=?`, status, id)
	return err
}

func (s *ExecutionStore) UpdateResult(id string, result string, status model.ExecutionStatus) error {
	_, err := s.db.Exec(`UPDATE bee_executions SET result=?, status=?, completed_at=? WHERE id=?`, result, status, time.Now().UnixMilli(), id)
	return err
}

func (s *ExecutionStore) UpdatePID(id string, pid int) error {
	_, err := s.db.Exec(`UPDATE bee_executions SET ai_process_pid=?, status=? WHERE id=?`, pid, model.ExecStatusRunning, id)
	return err
}

// MarkAbandoned finalizes an execution that exited without a terminal signal
// (killed, crashed, cancelled). Only updates if the row is still in pending
// or running state — terminal states (completed/failed) are left untouched.
// Returns whether a row was updated.
func (s *ExecutionStore) MarkAbandoned(ctx context.Context, id, result string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_executions SET status=?, result=?, completed_at=?
		 WHERE id=? AND status IN (?, ?)`,
		model.ExecStatusFailed, result, time.Now().UnixMilli(),
		id, model.ExecStatusPending, model.ExecStatusRunning,
	)
	if err != nil {
		return false, fmt.Errorf("mark abandoned: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ResetRunningExecutions finalizes all executions stuck in pending/running at
// startup. Any process they tracked is gone (we just rebooted), so the rows
// are orphans — mark them failed with a "server restarted" reason. Must be
// called synchronously during startup, before new work is dispatched.
func (s *ExecutionStore) ResetRunningExecutions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_executions SET status=?, result=?, completed_at=?
		 WHERE status IN (?, ?)`,
		model.ExecStatusFailed, "abandoned: server restarted", time.Now().UnixMilli(),
		model.ExecStatusPending, model.ExecStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("reset running executions: %w", err)
	}
	return res.RowsAffected()
}

// PrepareLogPath creates the date-partitioned log directory, records the log path in
// the DB, and returns the path. Must be called before launching the process so that
// the invoker can redirect stdout/stderr to the file.
// startedAt is used for date partitioning; falls back to time.Now() if nil.
func (s *ExecutionStore) PrepareLogPath(id string, startedAt *int64) (string, error) {
	var t time.Time
	if startedAt != nil {
		t = time.UnixMilli(*startedAt)
	} else {
		t = time.Now()
	}
	dateDir := filepath.Join(s.logsDir, t.Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(dateDir, id+".log")
	if _, err := s.db.Exec(`UPDATE bee_executions SET log_path=? WHERE id=?`, logPath, id); err != nil {
		return "", fmt.Errorf("set log_path: %w", err)
	}
	return logPath, nil
}

func scanExecutions(rows *sql.Rows) ([]model.WorkerExecution, error) {
	var execs []model.WorkerExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}
