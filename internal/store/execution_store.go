package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/model"
)

type ExecutionStore struct {
	db      *sql.DB
	logsDir string
}

func NewExecutionStore(db *sql.DB, logsDir string) *ExecutionStore {
	return &ExecutionStore{db: db, logsDir: logsDir}
}

func (s *ExecutionStore) Create(workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		WorkerID:     &workerID,
		SessionID:    sessionID,
		TriggerInput: triggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.WorkerID, exec.SessionID, exec.TriggerInput, exec.Status, millis,
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
		`INSERT INTO bee_executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, NULL, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.SessionID, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert bee execution: %w", err)
	}
	return exec, nil
}

const execSelect = `
SELECT e.id, e.worker_id, e.session_id, e.trigger_input, e.status, e.result, e.log_path,
       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
FROM bee_executions e
LEFT JOIN bee_workers w ON w.id = e.worker_id`

func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
	var e model.WorkerExecution
	err := scanner.Scan(&e.ID, &e.WorkerID, &e.SessionID, &e.TriggerInput, &e.Status, &e.Result, &e.LogPath, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
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

func (s *ExecutionStore) GetBySessionID(sessionID string) (model.WorkerExecution, error) {
	row := s.db.QueryRow(execSelect+` WHERE e.session_id = ?`, sessionID)
	e, err := scanExecution(row)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get execution by session: %w", err)
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

func (s *ExecutionStore) ListByWorkerID(workerID string) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(execSelect+` WHERE e.worker_id = ? ORDER BY e.started_at DESC`, workerID)
	if err != nil {
		return nil, fmt.Errorf("list executions by worker: %w", err)
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
	row := s.db.QueryRow(execSelect+` WHERE e.worker_id = ? AND e.status = 'running' LIMIT 1`, workerID)
	e, err := scanExecution(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running execution by worker: %w", err)
	}
	return &e, nil
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

// ReadLog returns the log content for an execution, reading from its log file.
// Returns an empty string (no error) when no log has been written yet.
func (s *ExecutionStore) ReadLog(id string) (string, error) {
	row := s.db.QueryRow(`SELECT log_path FROM bee_executions WHERE id = ?`, id)
	var logPath string
	if err := row.Scan(&logPath); err != nil {
		return "", fmt.Errorf("get log_path: %w", err)
	}
	if logPath == "" {
		return "", nil
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("read log file: %w", err)
	}
	return string(b), nil
}

// WriteLog writes content to a date-partitioned log file and records the path in the DB.
// startedAt is used to determine the date directory; falls back to time.Now() if nil.
func (s *ExecutionStore) WriteLog(id string, startedAt *int64, content string) (string, error) {
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
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write log file: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE bee_executions SET log_path=? WHERE id=?`, logPath, id); err != nil {
		return "", fmt.Errorf("update log_path: %w", err)
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

