package skill

import "time"

// SkillSource identifies whether a skill is openbee-managed or externally placed.
type SkillSource string

const (
	SkillSourceManaged  SkillSource = "managed"
	SkillSourceExternal SkillSource = "external"
)

// VersionEntry holds metadata for one version of a skill.
type VersionEntry struct {
	CreatedAt time.Time `json:"created_at"`
}

// SkillEntry is the registry record for one managed skill.
type SkillEntry struct {
	Description   string                  `json:"description"`
	LatestVersion string                  `json:"latest_version"`
	GlobalVersion string                  `json:"global_version"`
	Versions      map[string]VersionEntry `json:"versions"`
}

// SkillsConfig is the full contents of skills.json.
type SkillsConfig struct {
	Version int                          `json:"version"`
	Skills  map[string]SkillEntry        `json:"skills"`
	// WorkerOverrides maps workerID -> skillName -> version.
	// Absent entry means the worker inherits the global version.
	WorkerOverrides map[string]map[string]string `json:"worker_overrides"`
}

// ScannedSkill represents one skill found during a directory scan.
type ScannedSkill struct {
	Name          string
	Source        SkillSource
	ActiveVersion string // set for managed skills; empty for external
	IsOverride    bool   // true when a worker skill overrides a global one
	Scope         string // "global" or worker ID
	LinkTarget    string // resolved symlink target (empty if real directory)
}

// now returns the current UTC time truncated to seconds.
// Used by skill manager operations.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}
