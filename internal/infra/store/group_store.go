package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type GroupStore struct {
	db *sql.DB
}

func NewGroupStore(db *sql.DB) *GroupStore {
	return &GroupStore{db: db}
}

const groupColumns = `id, name, description, constraints, work_dir, engine, engine_args, status, permission_scopes, created_at, updated_at`

func scanGroup(scanner interface{ Scan(...any) error }) (model.Group, error) {
	var g model.Group
	err := scanner.Scan(
		&g.ID, &g.Name, &g.Description, &g.Constraints,
		&g.WorkDir, &g.Engine, &g.EngineArgs, &g.Status,
		&g.PermissionScopes, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

func scanGroups(rows *sql.Rows) ([]model.Group, error) {
	var out []model.Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *GroupStore) Create(g model.Group) (model.Group, error) {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	g.Status = model.WorkerStatusIdle
	g.CreatedAt = time.Now().UnixMilli()
	g.UpdatedAt = g.CreatedAt
	if g.EngineArgs == "" {
		g.EngineArgs = "{}"
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_groups (`+groupColumns+`)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.Description, g.Constraints,
		g.WorkDir, g.Engine, g.EngineArgs, g.Status,
		g.PermissionScopes, g.CreatedAt, g.UpdatedAt,
	)
	if err != nil {
		return model.Group{}, fmt.Errorf("insert group: %w", err)
	}
	return g, nil
}

func (s *GroupStore) GetByID(id string) (model.Group, error) {
	row := s.db.QueryRow(`SELECT `+groupColumns+` FROM bee_groups WHERE id = ?`, id)
	g, err := scanGroup(row)
	if err != nil {
		return model.Group{}, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

func (s *GroupStore) GetByName(name string) (model.Group, error) {
	row := s.db.QueryRow(
		`SELECT `+groupColumns+` FROM bee_groups
         WHERE LOWER(name) = LOWER(?)
         ORDER BY created_at ASC, ROWID ASC LIMIT 1`,
		name,
	)
	g, err := scanGroup(row)
	if err != nil {
		return model.Group{}, fmt.Errorf("get group by name: %w", err)
	}
	return g, nil
}

func (s *GroupStore) ExistsByName(name, excludeID string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM bee_groups WHERE LOWER(name) = LOWER(?) AND id != ?)`,
		name, excludeID,
	).Scan(&n)
	return n == 1, err
}

func (s *GroupStore) List() ([]model.Group, error) {
	rows, err := s.db.Query(`SELECT ` + groupColumns + ` FROM bee_groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (s *GroupStore) Update(g model.Group) (model.Group, error) {
	g.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_groups SET name=?, description=?, constraints=?, work_dir=?,
            engine=?, engine_args=?, status=?, permission_scopes=?, updated_at=?
         WHERE id=?`,
		g.Name, g.Description, g.Constraints, g.WorkDir,
		g.Engine, g.EngineArgs, g.Status, g.PermissionScopes, g.UpdatedAt,
		g.ID,
	)
	if err != nil {
		return model.Group{}, fmt.Errorf("update group: %w", err)
	}
	return g, nil
}

func (s *GroupStore) UpdateStatus(id string, status model.WorkerStatus) error {
	_, err := s.db.Exec(`UPDATE bee_groups SET status=?, updated_at=? WHERE id=?`, status, time.Now().UnixMilli(), id)
	return err
}

func (s *GroupStore) Delete(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM bee_worker_groups WHERE group_id = ?`, id); err != nil {
		return fmt.Errorf("delete memberships: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM bee_groups WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return tx.Commit()
}

func (s *GroupStore) AddMember(groupID, workerID, role string) error {
	if role == "" {
		role = "member"
	}
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO bee_worker_groups (worker_id, group_id, role, created_at)
         VALUES (?, ?, ?, ?)`,
		workerID, groupID, role, now,
	)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (s *GroupStore) RemoveMember(groupID, workerID string) error {
	_, err := s.db.Exec(
		`DELETE FROM bee_worker_groups WHERE group_id = ? AND worker_id = ?`,
		groupID, workerID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

func (s *GroupStore) IsMember(groupID, workerID string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM bee_worker_groups WHERE group_id = ? AND worker_id = ?)`,
		groupID, workerID,
	).Scan(&n)
	return n == 1, err
}

func (s *GroupStore) ListMembers(groupID string) ([]model.MemberBrief, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.name, w.description
         FROM bee_workers w
         JOIN bee_worker_groups wg ON wg.worker_id = w.id
         WHERE wg.group_id = ?
         ORDER BY w.created_at ASC`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var out []model.MemberBrief
	for rows.Next() {
		var m model.MemberBrief
		if err := rows.Scan(&m.ID, &m.Name, &m.Description); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *GroupStore) ListGroupsForWorker(workerID string) ([]model.GroupBrief, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.name
         FROM bee_groups g
         JOIN bee_worker_groups wg ON wg.group_id = g.id
         WHERE wg.worker_id = ?
         ORDER BY g.created_at ASC`,
		workerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list groups for worker: %w", err)
	}
	defer rows.Close()
	var out []model.GroupBrief
	for rows.Next() {
		var b model.GroupBrief
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
