package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Constraint struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt int64  `json:"updated_at"`
}

type ConstraintStore struct {
	db *sql.DB
}

func NewConstraintStore(db *sql.DB) *ConstraintStore {
	return &ConstraintStore{db: db}
}

func (s *ConstraintStore) Save(scope, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO bee_constraints (id, scope, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		 WHERE bee_constraints.value != excluded.value`,
		uuid.New().String(), scope, key, value, now, now,
	)
	return err
}

func (s *ConstraintStore) Get(scope, key string) (*Constraint, error) {
	row := s.db.QueryRow(
		`SELECT key, value, updated_at FROM bee_constraints WHERE scope = ? AND key = ?`,
		scope, key,
	)
	var c Constraint
	err := row.Scan(&c.Key, &c.Value, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ConstraintStore) ListByScope(scope string, limit int) ([]Constraint, error) {
	rows, err := s.db.Query(
		`SELECT key, value, updated_at FROM bee_constraints WHERE scope = ? ORDER BY updated_at DESC LIMIT ?`,
		scope, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constraints []Constraint
	for rows.Next() {
		var c Constraint
		if err := rows.Scan(&c.Key, &c.Value, &c.UpdatedAt); err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, rows.Err()
}

func (s *ConstraintStore) Delete(scope, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM bee_constraints WHERE scope = ? AND key = ?`,
		scope, key,
	)
	return err
}
