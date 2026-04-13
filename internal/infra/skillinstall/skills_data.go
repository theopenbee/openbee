package skillinstall

import _ "embed"

//go:embed skills/openbee-bee/SKILL.md
var beeSkillMD string

//go:embed skills/openbee-bee/references/cli-reference.md
var beeCLIRef string

//go:embed skills/openbee-bee/references/session-management.md
var beeSessionRef string

//go:embed skills/openbee-bee/references/memory-management.md
var beeMemoryRef string

//go:embed skills/openbee-bee/references/system-status.md
var beeSystemStatusRef string

//go:embed skills/openbee-bee/references/entity-relationships.md
var beeEntityRef string

//go:embed skills/openbee-bee/references/task-scheduling.md
var beeTaskSchedulingRef string

//go:embed skills/openbee-worker/SKILL.md
var workerSkillMD string

//go:embed skills/openbee-worker/references/cli-reference.md
var workerCLIRef string

//go:embed skills/openbee-worker/references/entity-relationships.md
var workerEntityRef string

type skillDef struct {
	name       string
	content    string
	references map[string]string // filename -> content
}

var embeddedSkills = []skillDef{
	{
		name:    "openbee-bee",
		content: beeSkillMD,
		references: map[string]string{
			"cli-reference.md":       beeCLIRef,
			"session-management.md":  beeSessionRef,
			"memory-management.md":   beeMemoryRef,
			"system-status.md":       beeSystemStatusRef,
			"entity-relationships.md": beeEntityRef,
			"task-scheduling.md":     beeTaskSchedulingRef,
		},
	},
	{
		name:    "openbee-worker",
		content: workerSkillMD,
		references: map[string]string{
			"cli-reference.md":        workerCLIRef,
			"entity-relationships.md": workerEntityRef,
		},
	},
}
