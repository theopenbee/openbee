package store

import "testing"

func TestMigrationV47_TablesAndSeedRoles(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"bee_users", "bee_roles", "bee_role_permissions", "bee_user_roles"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}
	}

	var roleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_roles`).Scan(&roleCount); err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roleCount != 3 {
		t.Fatalf("expected 3 seed roles, got %d", roleCount)
	}

	var wildcard int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM bee_role_permissions WHERE role_id='sysrole_superadmin' AND permission='*'`,
	).Scan(&wildcard); err != nil {
		t.Fatalf("query superadmin permission: %v", err)
	}
	if wildcard != 1 {
		t.Fatalf("expected super-admin to have '*' permission")
	}
}
