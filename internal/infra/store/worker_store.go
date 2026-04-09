package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type WorkerStore struct {
	db *sql.DB
}

func NewWorkerStore(db *sql.DB) *WorkerStore {
	return &WorkerStore{db: db}
}

func (s *WorkerStore) Create(w model.Worker) (model.Worker, error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	w.Status = model.WorkerStatusIdle
	w.CreatedAt = time.Now().UnixMilli()
	w.UpdatedAt = w.CreatedAt

	_, err := s.db.Exec(
		`INSERT INTO bee_workers (id, name, description, memory, work_dir, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.Memory, w.WorkDir,
		w.Status, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, fmt.Errorf("insert worker: %w", err)
	}
	return w, nil
}

const workerColumns = `id, name, description, memory, work_dir, status, created_at, updated_at`

func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
	var w model.Worker
	err := scanner.Scan(
		&w.ID, &w.Name, &w.Description, &w.Memory,
		&w.WorkDir, &w.Status, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, err
	}
	return w, nil
}

// GetByName returns the worker with the given name.
// Name comparison is case-insensitive for ASCII characters.
// If multiple workers share the same name, the one created earliest is returned (deterministic by ROWID).
// Returns an error wrapping sql.ErrNoRows if no worker is found.
func (s *WorkerStore) GetByName(name string) (model.Worker, error) {
	row := s.db.QueryRow(
		`SELECT `+workerColumns+` FROM bee_workers
		 WHERE LOWER(name) = LOWER(?)
		 ORDER BY created_at ASC, ROWID ASC
		 LIMIT 1`,
		name,
	)
	w, err := scanWorker(row)
	if err != nil {
		return model.Worker{}, fmt.Errorf("get worker by name: %w", err)
	}
	return w, nil
}

func (s *WorkerStore) GetByID(id string) (model.Worker, error) {
	row := s.db.QueryRow(`SELECT `+workerColumns+` FROM bee_workers WHERE id = ?`, id)
	w, err := scanWorker(row)
	if err != nil {
		return model.Worker{}, fmt.Errorf("get worker: %w", err)
	}
	return w, nil
}

func scanWorkers(rows *sql.Rows) ([]model.Worker, error) {
	var workers []model.Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (s *WorkerStore) GetByIDs(ids []string) ([]model.Worker, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT `+workerColumns+` FROM bee_workers WHERE id IN (`+inPlaceholders(len(ids))+`)`,
		stringsToArgs(ids)...,
	)
	if err != nil {
		return nil, fmt.Errorf("get workers by ids: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

func (s *WorkerStore) GetByDepartmentID(deptID string) ([]model.Worker, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.name, w.description, w.memory, w.work_dir, w.status, w.created_at, w.updated_at
		 FROM bee_workers w
		 INNER JOIN bee_worker_departments wd ON w.id = wd.worker_id
		 WHERE wd.department_id = ?
		 ORDER BY w.created_at DESC`,
		deptID,
	)
	if err != nil {
		return nil, fmt.Errorf("get workers by department: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

func (s *WorkerStore) List() ([]model.Worker, error) {
	rows, err := s.db.Query(`SELECT ` + workerColumns + ` FROM bee_workers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}


func (s *WorkerStore) Update(w model.Worker) (model.Worker, error) {
	w.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_workers SET name=?, description=?, memory=?, work_dir=?, status=?, updated_at=?
		 WHERE id=?`,
		w.Name, w.Description, w.Memory, w.WorkDir,
		w.Status, w.UpdatedAt, w.ID,
	)
	if err != nil {
		return model.Worker{}, fmt.Errorf("update worker: %w", err)
	}
	return w, nil
}

func (s *WorkerStore) UpdateStatus(id string, status model.WorkerStatus) error {
	_, err := s.db.Exec(`UPDATE bee_workers SET status=?, updated_at=? WHERE id=?`, status, time.Now().UnixMilli(), id)
	return err
}

func (s *WorkerStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM bee_workers WHERE id=?`, id)
	return err
}

// CountByStatus returns a map of worker status to count.
func (s *WorkerStore) CountByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM bee_workers GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
