package model

// SystemConfig is a global key/value system setting.
type SystemConfig struct {
	Key       string `db:"key"`
	Value     string `db:"value"`
	UpdatedAt int64  `db:"updated_at"`
}

// SystemConfigKeyDefaultEngine is the key for the default bee engine.
const SystemConfigKeyDefaultEngine = "default_engine"

// SystemConfigKeyEngineArgsGlobal is the key for global engine args (applied to all workers).
const SystemConfigKeyEngineArgsGlobal = "engine_args_global"

// SystemConfigKeyEngineArgsBee is the key for bee-level engine args.
const SystemConfigKeyEngineArgsBee = "engine_args_bee"

// SystemConfigKeyLinearProjects is the key for the Linear project allow-list.
// Stored as a JSON-encoded array of project names.
const SystemConfigKeyLinearProjects = "linear_projects"

// SystemConfigKeyLinearStates is the key for the Linear workflow-state name
// allow-list. Stored as a JSON-encoded array of state name strings.
const SystemConfigKeyLinearStates = "linear_states"
