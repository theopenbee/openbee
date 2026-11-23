package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func moveModelArgs(tx *sql.Tx) error {
	if err := moveWorkerModelArgs(tx); err != nil {
		return err
	}
	pairs := [][2]string{
		{model.SystemConfigKeyEngineArgsGlobal, model.SystemConfigKeyEngineModelsGlobal},
		{model.SystemConfigKeyEngineArgsBee, model.SystemConfigKeyEngineModelsBee},
	}
	for _, pair := range pairs {
		if err := moveConfigModelArgs(tx, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

type workerEngineMaps struct {
	id, args, models string
}

func moveWorkerModelArgs(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, engine_args, engine_models FROM bee_workers`)
	if err != nil {
		return err
	}
	var workers []workerEngineMaps
	for rows.Next() {
		var w workerEngineMaps
		if err := rows.Scan(&w.id, &w.args, &w.models); err != nil {
			rows.Close()
			return err
		}
		workers = append(workers, w)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, w := range workers {
		args, models, ok := decodeEngineMaps(w.args, w.models)
		if !ok || !ai.MoveModelArgs(args, models) {
			continue
		}
		if _, err := tx.Exec(`UPDATE bee_workers SET engine_args = ?, engine_models = ? WHERE id = ?`,
			encodeEngineMap(args), encodeEngineMap(models), w.id); err != nil {
			return err
		}
	}
	return nil
}

func moveConfigModelArgs(tx *sql.Tx, argsKey, modelsKey string) error {
	argsValue, err := systemConfigValue(tx, argsKey)
	if err != nil {
		return err
	}
	modelsValue, err := systemConfigValue(tx, modelsKey)
	if err != nil {
		return err
	}
	args, models, ok := decodeEngineMaps(argsValue, modelsValue)
	if !ok || !ai.MoveModelArgs(args, models) {
		return nil
	}
	now := time.Now().UnixMilli()
	for _, entry := range [][2]string{{argsKey, encodeEngineMap(args)}, {modelsKey, encodeEngineMap(models)}} {
		if _, err := tx.Exec(`INSERT INTO bee_system_configs (key, value, updated_at) VALUES (?, ?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			entry[0], entry[1], now); err != nil {
			return err
		}
	}
	return nil
}

func systemConfigValue(tx *sql.Tx, key string) (string, error) {
	var value string
	err := tx.QueryRow(`SELECT value FROM bee_system_configs WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func decodeEngineMaps(argsJSON, modelsJSON string) (args, models map[string]string, ok bool) {
	args, models = map[string]string{}, map[string]string{}
	if !decodeEngineMap(argsJSON, args) || !decodeEngineMap(modelsJSON, models) {
		return nil, nil, false
	}
	return args, models, len(args) > 0
}

func decodeEngineMap(raw string, into map[string]string) bool {
	if raw == "" {
		return true
	}
	return json.Unmarshal([]byte(raw), &into) == nil
}

func encodeEngineMap(entries map[string]string) string {
	b, _ := json.Marshal(entries)
	return string(b)
}
