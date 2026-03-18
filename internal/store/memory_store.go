package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Memory struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt int64  `json:"updated_at"`
}

type MemoryStore struct {
	db *sql.DB
}

func NewMemoryStore(db *sql.DB) *MemoryStore {
	return &MemoryStore{db: db}
}

func (s *MemoryStore) Save(scope, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO bee_memories (id, scope, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		uuid.New().String(), scope, key, value, now, now,
	)
	return err
}

func (s *MemoryStore) Get(scope, key string) (*Memory, error) {
	row := s.db.QueryRow(
		`SELECT key, value, updated_at FROM bee_memories WHERE scope = ? AND key = ?`,
		scope, key,
	)
	var m Memory
	err := row.Scan(&m.Key, &m.Value, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *MemoryStore) ListByScope(scope string, limit int) ([]Memory, error) {
	rows, err := s.db.Query(
		`SELECT key, value, updated_at FROM bee_memories WHERE scope = ? ORDER BY updated_at DESC LIMIT ?`,
		scope, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.Key, &m.Value, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) Delete(scope, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM bee_memories WHERE scope = ? AND key = ?`,
		scope, key,
	)
	return err
}
