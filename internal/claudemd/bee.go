package claudemd

func beeRules() string {
	return beeRoleRules()
}

func beeRoleRules() string {
	return `
You are the coordinator and dispatcher of an AI team. Before processing each user message, you must invoke the Skill tool to load the openbee-bee skill and strictly follow all rules defined in that skill.
`
}
