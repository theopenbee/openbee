package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationV47_TablesAndSeedRoles(t *testing.T) {
	db := newTestDB(t)

	for _, table := range []string{"bee_users", "bee_roles", "bee_role_permissions", "bee_user_roles"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		require.NoError(t, err, "expected table %s to exist", table)
	}

	var roleCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_roles`).Scan(&roleCount))
	require.Equal(t, 1, roleCount)

	var wildcard int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM bee_role_permissions WHERE role_id='sysrole_superadmin' AND permission='*'`,
	).Scan(&wildcard))
	require.Equal(t, 1, wildcard, "expected super-admin to have '*' permission")
}
