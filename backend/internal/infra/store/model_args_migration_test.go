package store

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration_MovesModelArgsToEngineModels(t *testing.T) {
	db := openPreMigrationDB(t, 62)

	workers := []struct{ id, args, models string }{
		{"moved", `{"claude":"--model opus --effort high","pi":"--model=deepseek/deepseek-flash"}`, `{}`},
		{"kept", `{"claude":"--model sonnet"}`, `{"claude":"opus"}`},
		{"short", `{"codex":"-m o3"}`, `{}`},
		{"bad", `{"pi":"--model --verbose"}`, `{}`},
		{"none", `{"claude":"--effort high"}`, `{"pi":"x"}`},
		{"broken", `{`, `{}`},
	}
	for _, w := range workers {
		_, err := db.Exec(seedWorkerSQL(w.id))
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE bee_workers SET engine_args = ?, engine_models = ? WHERE id = ?`, w.args, w.models, w.id)
		require.NoError(t, err)
	}
	_, err := db.Exec(`INSERT INTO bee_system_configs (key, value, updated_at) VALUES
		('engine_args_global', '{"opencode":"--model deepseek/deepseek-v4-pro --print-logs","pi":"--thinking high"}', 1),
		('engine_args_bee', '{"claude":"--model \"opus\""}', 1)`)
	require.NoError(t, err)

	require.NoError(t, migrate(db))

	want := map[string][2]string{
		"moved":  {`{"claude":"--effort high"}`, `{"claude":"opus","pi":"deepseek/deepseek-flash"}`},
		"kept":   {`{"claude":"--model sonnet"}`, `{"claude":"opus"}`},
		"short":  {`{"codex":"-m o3"}`, `{}`},
		"bad":    {`{"pi":"--model --verbose"}`, `{}`},
		"none":   {`{"claude":"--effort high"}`, `{"pi":"x"}`},
		"broken": {`{`, `{}`},
	}
	for id, pair := range want {
		var args, models string
		require.NoError(t, db.QueryRow(`SELECT engine_args, engine_models FROM bee_workers WHERE id = ?`, id).Scan(&args, &models))
		assert.Equal(t, pair[0], args, id)
		assert.Equal(t, pair[1], models, id)
	}
	assert.JSONEq(t, `{"opencode":"--print-logs","pi":"--thinking high"}`, configValueOf(t, db, "engine_args_global"))
	assert.JSONEq(t, `{"opencode":"deepseek/deepseek-v4-pro"}`, configValueOf(t, db, "engine_models_global"))
	assert.JSONEq(t, `{}`, configValueOf(t, db, "engine_args_bee"))
	assert.JSONEq(t, `{"claude":"opus"}`, configValueOf(t, db, "engine_models_bee"))
}

func configValueOf(t *testing.T, db *sql.DB, key string) string {
	t.Helper()
	var value string
	require.NoError(t, db.QueryRow(`SELECT value FROM bee_system_configs WHERE key = ?`, key).Scan(&value))
	return value
}
