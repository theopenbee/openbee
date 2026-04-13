package skillinstall

import "embed"

//go:embed skills
var skillsFS embed.FS

var embeddedSkills = []string{"openbee-bee", "openbee-worker"}
