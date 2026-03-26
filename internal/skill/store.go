package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store reads and writes the skills.json state file.
type Store struct {
	path string
}

// NewStore returns a Store for the given JSON file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads the config from disk. Returns an empty config if the file does not exist.
func (s *Store) Load() (SkillsConfig, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return SkillsConfig{
			Version:         1,
			Skills:          make(map[string]SkillEntry),
			WorkerOverrides: make(map[string]map[string]string),
		}, nil
	}
	if err != nil {
		return SkillsConfig{}, err
	}

	var cfg SkillsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return SkillsConfig{}, err
	}
	if cfg.Skills == nil {
		cfg.Skills = make(map[string]SkillEntry)
	}
	if cfg.WorkerOverrides == nil {
		cfg.WorkerOverrides = make(map[string]map[string]string)
	}
	return cfg, nil
}

// Save writes the config atomically (write to temp file, then rename).
func (s *Store) Save(cfg SkillsConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	defer func() { _ = os.Remove(tmp) }()
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
