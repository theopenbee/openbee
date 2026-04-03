package skillinstall

import _ "embed"

//go:embed skills/openbee-bee/SKILL.md
var beeSkillMD string

//go:embed skills/openbee-worker/SKILL.md
var workerSkillMD string

type skillDef struct {
	name    string
	content string
}

var embeddedSkills = []skillDef{
	{name: "openbee-bee", content: beeSkillMD},
	{name: "openbee-worker", content: workerSkillMD},
}
