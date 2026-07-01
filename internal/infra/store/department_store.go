package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type DepartmentStore struct {
	db *sql.DB
}

func NewDepartmentStore(db *sql.DB) *DepartmentStore {
	return &DepartmentStore{db: db}
}

const departmentColumns = `id, name, parent_id, sort_order, created_at, updated_at`

func scanDepartment(scanner interface{ Scan(...any) error }) (model.Department, error) {
	var d model.Department
	err := scanner.Scan(&d.ID, &d.Name, &d.ParentID, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func scanDepartments(rows *sql.Rows) ([]model.Department, error) {
	var departments []model.Department
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, err
		}
		departments = append(departments, d)
	}
	return departments, rows.Err()
}

func (s *DepartmentStore) Create(d model.Department) (model.Department, error) {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UnixMilli()
	d.CreatedAt = now
	d.UpdatedAt = now

	if d.ParentID != nil {
		if _, err := s.GetByID(*d.ParentID); err != nil {
			return model.Department{}, apperr.New("department_parent_not_found", i18n.M.Runtime.Department.ParentNotFound)
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO bee_departments (id, name, parent_id, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.Name, d.ParentID, d.SortOrder, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return model.Department{}, fmt.Errorf("insert department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) GetByID(id string) (model.Department, error) {
	row := s.db.QueryRow(`SELECT `+departmentColumns+` FROM bee_departments WHERE id = ?`, id)
	d, err := scanDepartment(row)
	if err != nil {
		return model.Department{}, fmt.Errorf("get department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) GetByIDs(ids []string) ([]model.Department, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT `+departmentColumns+` FROM bee_departments WHERE id IN (`+inPlaceholders(len(ids))+`)`,
		stringsToArgs(ids)...,
	)
	if err != nil {
		return nil, fmt.Errorf("get departments by ids: %w", err)
	}
	defer rows.Close()
	return scanDepartments(rows)
}

func (s *DepartmentStore) ListAll() ([]model.Department, error) {
	rows, err := s.db.Query(`SELECT ` + departmentColumns + ` FROM bee_departments ORDER BY sort_order, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	defer rows.Close()
	return scanDepartments(rows)
}

func (s *DepartmentStore) Update(d model.Department) (model.Department, error) {
	d.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_departments SET name=?, parent_id=?, sort_order=?, updated_at=? WHERE id=?`,
		d.Name, d.ParentID, d.SortOrder, d.UpdatedAt, d.ID,
	)
	if err != nil {
		return model.Department{}, fmt.Errorf("update department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) Delete(id string) error {
	hasChildren, err := s.HasChildren(id)
	if err != nil {
		return err
	}
	if hasChildren {
		return apperr.New("department_has_sub", i18n.M.Runtime.Department.HasSubDepartments)
	}

	hasWorkers, err := s.HasWorkers(id)
	if err != nil {
		return err
	}
	if hasWorkers {
		return apperr.New("department_has_workers", i18n.M.Runtime.Department.HasAssociatedWorkers)
	}

	_, err = s.db.Exec(`DELETE FROM bee_departments WHERE id=?`, id)
	return err
}

func (s *DepartmentStore) HasChildren(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bee_departments WHERE parent_id = ?)`, id).Scan(&exists)
	return exists, err
}

func (s *DepartmentStore) HasWorkers(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bee_worker_departments WHERE department_id = ?)`, id).Scan(&exists)
	return exists, err
}

// CheckCircularReference returns an error if setting parentID as the parent of
// departmentID would create a cycle.
func (s *DepartmentStore) CheckCircularReference(departmentID, parentID string) error {
	var found int
	err := s.db.QueryRow(`
		WITH RECURSIVE ancestors(id, parent_id) AS (
			SELECT id, parent_id FROM bee_departments WHERE id = ?
			UNION ALL
			SELECT d.id, d.parent_id FROM bee_departments d JOIN ancestors a ON d.id = a.parent_id
		)
		SELECT COUNT(*) FROM ancestors WHERE id = ?`, parentID, departmentID,
	).Scan(&found)
	if err != nil {
		return nil // broken chain, no cycle
	}
	if found > 0 {
		return apperr.New("department_circular_reference", i18n.M.Runtime.Department.CircularReference)
	}
	return nil
}

func (s *DepartmentStore) BuildTree(depts []model.Department) []model.DepartmentTree {
	childrenMap := make(map[string][]model.Department, len(depts))
	for _, d := range depts {
		if d.ParentID != nil {
			childrenMap[*d.ParentID] = append(childrenMap[*d.ParentID], d)
		}
	}

	var buildNode func(d model.Department) model.DepartmentTree
	buildNode = func(d model.Department) model.DepartmentTree {
		node := model.DepartmentTree{Department: d, Children: []model.DepartmentTree{}}
		for _, child := range childrenMap[d.ID] {
			node.Children = append(node.Children, buildNode(child))
		}
		return node
	}

	var roots []model.DepartmentTree
	for _, d := range depts {
		if d.ParentID == nil {
			roots = append(roots, buildNode(d))
		}
	}

	sortTreeSlice(roots)
	return roots
}

func sortTreeSlice(nodes []model.DepartmentTree) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].SortOrder != nodes[j].SortOrder {
			return nodes[i].SortOrder < nodes[j].SortOrder
		}
		return nodes[i].CreatedAt < nodes[j].CreatedAt
	})
	for i := range nodes {
		sortTreeSlice(nodes[i].Children)
	}
}

// SetWorkerDepartments replaces all department associations for a worker.
func (s *DepartmentStore) SetWorkerDepartments(workerID string, deptIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID); err != nil {
		return fmt.Errorf("clear worker departments: %w", err)
	}

	if len(deptIDs) > 0 {
		now := time.Now().UnixMilli()
		args := make([]any, 0, len(deptIDs)*3)
		for _, deptID := range deptIDs {
			args = append(args, workerID, deptID, now)
		}
		query := `INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES ` +
			strings.Repeat("(?, ?, ?),", len(deptIDs))
		query = query[:len(query)-1] // trim trailing comma
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("insert worker departments: %w", err)
		}
	}

	return tx.Commit()
}

const departmentColumnsAliased = `d.id, d.name, d.parent_id, d.sort_order, d.created_at, d.updated_at`

// GetWorkerDepartments returns all departments a worker belongs to.
func (s *DepartmentStore) GetWorkerDepartments(workerID string) ([]model.Department, error) {
	rows, err := s.db.Query(
		`SELECT `+departmentColumnsAliased+` FROM bee_departments d
		 INNER JOIN bee_worker_departments wd ON d.id = wd.department_id
		 WHERE wd.worker_id = ?
		 ORDER BY d.sort_order, d.created_at`,
		workerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get worker departments: %w", err)
	}
	defer rows.Close()
	return scanDepartments(rows)
}

// GetWorkersDepartments returns a map of workerID → departments for the given worker IDs.
func (s *DepartmentStore) GetWorkersDepartments(workerIDs []string) (map[string][]model.Department, error) {
	if len(workerIDs) == 0 {
		return map[string][]model.Department{}, nil
	}

	query := `SELECT wd.worker_id, ` + departmentColumnsAliased + ` FROM bee_departments d
		 INNER JOIN bee_worker_departments wd ON d.id = wd.department_id
		 WHERE wd.worker_id IN (` + inPlaceholders(len(workerIDs)) + `)
		 ORDER BY d.sort_order, d.created_at`

	rows, err := s.db.Query(query, stringsToArgs(workerIDs)...)
	if err != nil {
		return nil, fmt.Errorf("get worker departments map: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]model.Department)
	for rows.Next() {
		var workerID string
		var d model.Department
		if err := rows.Scan(&workerID, &d.ID, &d.Name, &d.ParentID, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		result[workerID] = append(result[workerID], d)
	}
	return result, rows.Err()
}

// GetWorkerIDsForDepartments returns the unique worker IDs associated with any of the given departments.
func (s *DepartmentStore) GetWorkerIDsForDepartments(deptIDs []string) ([]string, error) {
	if len(deptIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT DISTINCT worker_id FROM bee_worker_departments WHERE department_id IN (`+inPlaceholders(len(deptIDs))+`)`,
		stringsToArgs(deptIDs)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteWorkerDepartments removes all department associations for a worker.
func (s *DepartmentStore) DeleteWorkerDepartments(workerID string) error {
	_, err := s.db.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID)
	return err
}
