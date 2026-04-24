package model

// SystemConfig is a global key/value system setting.
type SystemConfig struct {
	Key       string `db:"key"`
	Value     string `db:"value"`
	UpdatedAt int64  `db:"updated_at"`
}

// SystemConfigKeyDefaultEngine is the key for the default bee engine.
const SystemConfigKeyDefaultEngine = "default_engine"

// SystemConfigKeyEngineExtraArgsGlobal is the key for global engine extra args (applied to all workers).
const SystemConfigKeyEngineExtraArgsGlobal = "engine_extra_args_global"

// SystemConfigKeyEngineExtraArgsBee is the key for bee-level engine extra args.
const SystemConfigKeyEngineExtraArgsBee = "engine_extra_args_bee"
