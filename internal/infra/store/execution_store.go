package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// LogSlice is the result of a byte-offset log read. Size reports the file's
// current byte length; callers use it as the next "since" offset. Truncated
// is true when the caller's since exceeded Size (file rotated/reset), in
// which case Content holds the full file and callers should rebuild state.
// Status is the execution's current status, returned alongside the slice so
// callers (e.g. log viewers) can decide whether to keep polling.
type LogSlice struct {
	Content   string
	Size      int64
	Truncated bool
	Status    model.ExecutionStatus
}

type ExecutionStore struct {
	db      *sql.DB
	logsDir string
}

func NewExecutionStore(db *sql.DB, logsDir string) *ExecutionStore {
	return &ExecutionStore{db: db, logsDir: logsDir}
}

func (s *ExecutionStore) Create(workerID, triggerInput, sessionID, engine string) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		WorkerID:     &workerID,
		SessionID:    sessionID,
		Engine:       engine,
		TriggerInput: triggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.WorkerID, exec.SessionID, exec.Engine, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert execution: %w", err)
	}
	return exec, nil
}

func (s *ExecutionStore) CreateBeeExecution(sessionID, triggerInput string) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		WorkerID:     nil, // bee execution — no worker
		SessionID:    sessionID,
		TriggerInput: triggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, NULL, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.SessionID, exec.Engine, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert bee execution: %w", err)
	}
	return exec, nil
}

const execSelect = `
SELECT e.id, e.worker_id, e.session_id, e.engine, e.trigger_input, e.status, e.result, e.log_path,
       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
FROM bee_executions e
LEFT JOIN bee_workers w ON w.id = e.worker_id`

func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
	var e model.WorkerExecution
	err := scanner.Scan(&e.ID, &e.WorkerID, &e.SessionID, &e.Engine, &e.TriggerInput, &e.Status, &e.Result, &e.LogPath, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
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

// CountSessions returns the total number of distinct sessions.
func (s *ExecutionStore) CountSessions() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM bee_executions`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

// ListPaginated returns executions grouped by session with pagination at the session level.
func (s *ExecutionStore) ListPaginated(limit, offset int) ([]model.WorkerExecution, error) {
	query := execSelect + ` WHERE e.session_id IN (
		SELECT session_id FROM bee_executions
		GROUP BY session_id
		ORDER BY MAX(started_at) DESC
		LIMIT ? OFFSET ?
	) ORDER BY e.started_at DESC`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list paginated executions: %w", err)
	}
	defer rows.Close()
	return scanExecutions(rows)
}

func (s *ExecutionStore) ListBySessionID(sessionID string) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(execSelect+` WHERE e.session_id = ? ORDER BY e.started_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list executions by session: %w", err)
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// CountSessionsByWorkerID returns the total number of distinct sessions for a worker.
func (s *ExecutionStore) CountSessionsByWorkerID(workerID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM bee_executions WHERE worker_id = ?`, workerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sessions by worker: %w", err)
	}
	return count, nil
}

// ListPaginatedByWorkerID returns executions for a worker grouped by session with pagination at the session level.
func (s *ExecutionStore) ListPaginatedByWorkerID(workerID string, limit, offset int) ([]model.WorkerExecution, error) {
	query := execSelect + ` WHERE e.worker_id = ? AND e.session_id IN (
		SELECT session_id FROM bee_executions WHERE worker_id = ?
		GROUP BY session_id
		ORDER BY MAX(started_at) DESC
		LIMIT ? OFFSET ?
	) ORDER BY e.started_at DESC`
	rows, err := s.db.Query(query, workerID, workerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list paginated executions by worker: %w", err)
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

// HasActiveExecutions reports whether any executions with status pending or running exist.
func (s *ExecutionStore) HasActiveExecutions(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_executions WHERE status IN (?, ?))`,
		model.ExecStatusPending, model.ExecStatusRunning,
	).Scan(&exists)
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

// ReadLogSince returns the log bytes at or after the given offset, along with
// the execution's current status (so callers can avoid a separate GetByID).
//   - since <= 0: returns full file
//   - since == Size: returns empty content (caller is caught up)
//   - since > Size: returns full content with Truncated=true (file was rotated/reset)
//
// Returns a LogSlice with empty content (and the current status) when no log
// path is set or the file does not yet exist.
func (s *ExecutionStore) ReadLogSince(id string, since int64) (LogSlice, error) {
	var logPath string
	var status model.ExecutionStatus
	if err := s.db.QueryRow(`SELECT log_path, status FROM bee_executions WHERE id = ?`, id).Scan(&logPath, &status); err != nil {
		return LogSlice{}, fmt.Errorf("get log_path: %w", err)
	}
	if logPath == "" {
		return LogSlice{Status: status}, nil
	}
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return LogSlice{Status: status}, nil
		}
		return LogSlice{}, fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return LogSlice{}, fmt.Errorf("stat log file: %w", err)
	}
	size := info.Size()

	// since out of range — file shrank/rotated; rebuild from scratch.
	if since > size {
		b, err := io.ReadAll(f)
		if err != nil {
			return LogSlice{}, fmt.Errorf("read log file: %w", err)
		}
		return LogSlice{Content: string(b), Size: size, Truncated: true, Status: status}, nil
	}

	// caller up to date.
	if since == size {
		return LogSlice{Size: size, Status: status}, nil
	}

	if since > 0 {
		if _, err := f.Seek(since, io.SeekStart); err != nil {
			return LogSlice{}, fmt.Errorf("seek log file: %w", err)
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return LogSlice{}, fmt.Errorf("read log file: %w", err)
	}
	return LogSlice{Content: string(b), Size: size, Status: status}, nil
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

// ExecutionFilter holds optional filter criteria for ListFiltered.
// Zero/empty values are ignored (no filtering on that field).
type ExecutionFilter struct {
	WorkerID      string
	SessionID     string
	Status        string
	StartedFrom   int64 // inclusive lower bound (Unix ms); 0 = no lower bound
	StartedTo     int64 // inclusive upper bound (Unix ms); 0 = no upper bound
	CompletedFrom int64 // inclusive lower bound (Unix ms); 0 = no lower bound
	CompletedTo   int64 // inclusive upper bound (Unix ms); 0 = no upper bound
}

// ListFiltered returns paginated executions matching the given filters and the total count.
func (s *ExecutionStore) ListFiltered(ctx context.Context, f ExecutionFilter, limit, offset int) ([]model.WorkerExecution, int, error) {
	where, args := executionFilterWhere(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bee_executions e"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count filtered executions: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, execSelect+where+" ORDER BY e.started_at DESC LIMIT ? OFFSET ?", appendPaginationArgs(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list filtered executions: %w", err)
	}
	defer rows.Close()
	execs, err := scanExecutions(rows)
	return execs, total, err
}

func executionFilterWhere(f ExecutionFilter) (string, []any) {
	var b whereBuilder
	if f.WorkerID != "" {
		b.add("e.worker_id = ?", f.WorkerID)
	}
	if f.SessionID != "" {
		b.add("e.session_id = ?", f.SessionID)
	}
	if f.Status != "" {
		b.add("e.status = ?", f.Status)
	}
	if f.StartedFrom > 0 {
		b.add("e.started_at >= ?", f.StartedFrom)
	}
	if f.StartedTo > 0 {
		b.add("e.started_at <= ?", f.StartedTo)
	}
	if f.CompletedFrom > 0 {
		b.add("e.completed_at >= ?", f.CompletedFrom)
	}
	if f.CompletedTo > 0 {
		b.add("e.completed_at <= ?", f.CompletedTo)
	}
	return b.build()
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

// ListBeeExecutions returns the bee's own execution history (worker_id IS NULL).
func (s *ExecutionStore) ListBeeExecutions(limit int) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(
		execSelect+` WHERE e.worker_id IS NULL ORDER BY e.started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// ListRecent returns the most recent executions (all types).
func (s *ExecutionStore) ListRecent(limit int) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(
		execSelect+` ORDER BY e.started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}
