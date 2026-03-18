package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/model"
)

type ExecutionStore struct {
	db *sql.DB
}

func NewExecutionStore(db *sql.DB) *ExecutionStore {
	return &ExecutionStore{db: db}
}

func (s *ExecutionStore) Create(workerID, triggerInput, sessionID string) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		WorkerID:     workerID,
		SessionID:    sessionID,
		TriggerInput: triggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	_, err := s.db.Exec(
		`INSERT INTO worker_executions (id, worker_id, session_id, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.WorkerID, exec.SessionID, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert execution: %w", err)
	}
	return exec, nil
}

const execSelect = `
SELECT e.id, e.worker_id, e.session_id, e.trigger_input, e.status, e.result, e.logs,
       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
FROM worker_executions e
LEFT JOIN workers w ON w.id = e.worker_id`

func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
	var e model.WorkerExecution
	err := scanner.Scan(&e.ID, &e.WorkerID, &e.SessionID, &e.TriggerInput, &e.Status, &e.Result, &e.Logs, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
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

	var execs []model.WorkerExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (s *ExecutionStore) ListBySessionID(sessionID string) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(execSelect+` WHERE e.session_id = ? ORDER BY e.started_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list executions by session: %w", err)
	}
	defer rows.Close()

	var execs []model.WorkerExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (s *ExecutionStore) ListByWorkerID(workerID string) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(execSelect+` WHERE e.worker_id = ? ORDER BY e.started_at DESC`, workerID)
	if err != nil {
		return nil, fmt.Errorf("list executions by worker: %w", err)
	}
	defer rows.Close()

	var execs []model.WorkerExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (s *ExecutionStore) UpdateStatus(id string, status model.ExecutionStatus) error {
	_, err := s.db.Exec(`UPDATE worker_executions SET status=? WHERE id=?`, status, id)
	return err
}

func (s *ExecutionStore) UpdateLogs(id string, logs string) error {
	_, err := s.db.Exec(`UPDATE worker_executions SET logs=? WHERE id=?`, logs, id)
	return err
}

func (s *ExecutionStore) UpdateResult(id string, result string, status model.ExecutionStatus) error {
	_, err := s.db.Exec(`UPDATE worker_executions SET result=?, status=?, completed_at=? WHERE id=?`, result, status, time.Now().UnixMilli(), id)
	return err
}

func (s *ExecutionStore) UpdatePID(id string, pid int) error {
	_, err := s.db.Exec(`UPDATE worker_executions SET ai_process_pid=?, status=? WHERE id=?`, pid, model.ExecStatusRunning, id)
	return err
}
