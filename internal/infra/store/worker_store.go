package store

import (
	"context"
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
		`INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine,
		w.Status, w.PermissionScopes, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, fmt.Errorf("insert worker: %w", err)
	}
	return w, nil
}

const (
	workerColumns        = `id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at`
	workerColumnsAliased = `w.id, w.name, w.description, w.constraints, w.work_dir, w.engine, w.status, w.permission_scopes, w.created_at, w.updated_at`
)

func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
	var w model.Worker
	err := scanner.Scan(
		&w.ID, &w.Name, &w.Description, &w.Constraints,
		&w.WorkDir, &w.Engine, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, err
	}
	return w, nil
}

func (s *WorkerStore) ListByName(name string) ([]model.Worker, error) {
	rows, err := s.db.Query(
		`SELECT `+workerColumns+` FROM bee_workers
		 WHERE LOWER(name) = LOWER(?)
		 ORDER BY created_at ASC, ROWID ASC`,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("list workers by name: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

func (s *WorkerStore) ExistsByName(name, excludeID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM bee_workers WHERE LOWER(TRIM(name)) = LOWER(TRIM(?)) AND id != ?`,
		name, excludeID,
	).Scan(&count)
	return count > 0, err
}

// When names collide, the earliest-created worker is returned.
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
		`SELECT `+workerColumnsAliased+`
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

func (s *WorkerStore) ListNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM bee_workers`)
	if err != nil {
		return nil, fmt.Errorf("list worker names: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// WorkerFilter holds optional filter criteria for listing workers.
type WorkerFilter struct {
	Name      string   // case-insensitive partial match; empty means no filter
	ID        string   // exact match; empty means no filter
	WorkerIDs []string // nil means no restriction; non-nil (even empty) restricts to these IDs
}

// ListFiltered returns workers matching the filter with pagination, plus the total count.
func (s *WorkerStore) ListFiltered(ctx context.Context, filter WorkerFilter, limit, offset int) ([]model.Worker, int, error) {
	if filter.WorkerIDs != nil && len(filter.WorkerIDs) == 0 {
		return []model.Worker{}, 0, nil
	}

	var b whereBuilder
	if filter.ID != "" {
		b.add("id = ?", filter.ID)
	}
	if filter.Name != "" {
		b.add("LOWER(name) LIKE LOWER(?)", "%"+filter.Name+"%")
	}
	if filter.WorkerIDs != nil {
		b.addAll("id IN ("+inPlaceholders(len(filter.WorkerIDs))+")", stringsToArgs(filter.WorkerIDs)...)
	}
	where, args := b.build()

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bee_workers"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count workers: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+workerColumns+" FROM bee_workers"+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		appendPaginationArgs(args, limit, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list workers filtered: %w", err)
	}
	defer rows.Close()
	workers, err := scanWorkers(rows)
	return workers, total, err
}

func (s *WorkerStore) Update(w model.Worker) (model.Worker, error) {
	w.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_workers SET name=?, description=?, constraints=?, work_dir=?, engine=?, status=?, permission_scopes=?, updated_at=?
		 WHERE id=?`,
		w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine,
		w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
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

func (s *WorkerStore) UpdateEngine(id, engine string) error {
	_, err := s.db.Exec(`UPDATE bee_workers SET engine=?, updated_at=? WHERE id=?`, engine, time.Now().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("update worker engine: %w", err)
	}
	return nil
}

func (s *WorkerStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM bee_workers WHERE id=?`, id)
	return err
}

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
