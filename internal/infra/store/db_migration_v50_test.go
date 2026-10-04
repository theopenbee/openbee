package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pins the DSN fix: modernc.org/sqlite ignored the old mattn-style keys, so
// none of these pragmas were in effect.
func TestInitDB_PragmasApplied(t *testing.T) {
	db := newTestDB(t)
	for pragma, want := range map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
	} {
		var got string
		require.NoError(t, db.QueryRow("PRAGMA "+pragma).Scan(&got), pragma)
		assert.Equal(t, want, got, pragma)
	}
}

func TestMigrationV50_PurgesOrphanedLinkRows(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"),
		`INSERT INTO bee_departments (id, name, created_at, updated_at) VALUES ('d1', 'D', 1, 1)`,
		`INSERT INTO bee_users (id, username, password_hash, created_at, updated_at) VALUES ('u1', 'u1', 'x', 1, 1)`,
		`INSERT INTO bee_roles (id, name, created_at, updated_at) VALUES ('r1', 'r1', 1, 1)`,
		`INSERT INTO bee_role_permissions (role_id, permission) VALUES ('r1', 'tasks:read')`,
		`INSERT INTO bee_user_roles (user_id, role_id, created_at) VALUES ('u1', 'r1', 1)`,
		`INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES ('w1', 'd1', 1)`,
	)
	// Recreate the orphans that pre-v50 databases accumulated while
	// foreign_keys was silently off (single connection, so the pragma sticks).
	for _, q := range []string{
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO bee_role_permissions (role_id, permission) VALUES ('gone-role', '*')`,
		`INSERT INTO bee_user_roles (user_id, role_id, created_at) VALUES ('u1', 'gone-role', 1)`,
		`INSERT INTO bee_user_roles (user_id, role_id, created_at) VALUES ('gone-user', 'r1', 1)`,
		`INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES ('gone-worker', 'd1', 1)`,
		`INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES ('w1', 'gone-dept', 1)`,
		`PRAGMA foreign_keys = ON`,
		`DELETE FROM bee_migrations WHERE version = 50`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, migrate(db))

	for _, tc := range []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM bee_role_permissions WHERE role_id = 'r1'`, 1},
		{`SELECT COUNT(*) FROM bee_role_permissions WHERE role_id = 'gone-role'`, 0},
		{`SELECT COUNT(*) FROM bee_user_roles WHERE user_id = 'u1' AND role_id = 'r1'`, 1},
		{`SELECT COUNT(*) FROM bee_user_roles`, 1},
		{`SELECT COUNT(*) FROM bee_worker_departments WHERE worker_id = 'w1' AND department_id = 'd1'`, 1},
		{`SELECT COUNT(*) FROM bee_worker_departments`, 1},
	} {
		var got int
		require.NoError(t, db.QueryRow(tc.query).Scan(&got), tc.query)
		assert.Equal(t, tc.want, got, tc.query)
	}
}
