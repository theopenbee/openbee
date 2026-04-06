package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
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
			return model.Department{}, fmt.Errorf("parent department not found")
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
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(
		`SELECT `+departmentColumns+` FROM bee_departments WHERE id IN (`+inPlaceholders(len(ids))+`)`,
		args...,
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
		return fmt.Errorf("department is not empty: has sub-departments")
	}

	hasWorkers, err := s.HasWorkers(id)
	if err != nil {
		return err
	}
	if hasWorkers {
		return fmt.Errorf("department is not empty: has associated workers")
	}

	_, err = s.db.Exec(`DELETE FROM bee_departments WHERE id=?`, id)
	return err
}

func (s *DepartmentStore) HasChildren(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_departments WHERE parent_id = ?`, id).Scan(&count)
	return count > 0, err
}

func (s *DepartmentStore) HasWorkers(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_worker_departments WHERE department_id = ?`, id).Scan(&count)
	return count > 0, err
}

// CheckCircularReference returns an error if setting parentID as the parent of
// departmentID would create a cycle.
func (s *DepartmentStore) CheckCircularReference(departmentID, parentID string) error {
	current := parentID
	for current != "" {
		if current == departmentID {
			return fmt.Errorf("circular reference detected")
		}
		d, err := s.GetByID(current)
		if err != nil {
			return nil // parent chain broken, no cycle
		}
		if d.ParentID == nil {
			break
		}
		current = *d.ParentID
	}
	return nil
}

// BuildTree assembles a flat list of departments into a tree structure.
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

	if _, err := tx.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("clear worker departments: %w", err)
	}

	now := time.Now().UnixMilli()
	for _, deptID := range deptIDs {
		if _, err := tx.Exec(
			`INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES (?, ?, ?)`,
			workerID, deptID, now,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert worker department: %w", err)
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

// GetAllWorkerDepartments returns a map of workerID → departments for all workers.
func (s *DepartmentStore) GetAllWorkerDepartments() (map[string][]model.Department, error) {
	rows, err := s.db.Query(
		`SELECT wd.worker_id, ` + departmentColumnsAliased + ` FROM bee_departments d
		 INNER JOIN bee_worker_departments wd ON d.id = wd.department_id
		 ORDER BY d.sort_order, d.created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all worker departments: %w", err)
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

// GetDepartmentWorkerIDs returns the IDs of workers directly associated with a department.
func (s *DepartmentStore) GetDepartmentWorkerIDs(deptID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT worker_id FROM bee_worker_departments WHERE department_id = ?`, deptID,
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
// Called when a worker is deleted.
func (s *DepartmentStore) DeleteWorkerDepartments(workerID string) error {
	_, err := s.db.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID)
	return err
}
